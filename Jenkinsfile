def buildParamPrefix = "BUILD_"
properties([
    parameters([
        booleanParam(
            name: "${buildParamPrefix}VELERO",
            defaultValue: true,
            description: "Build and push base velero image"
        ),
        booleanParam(
            name: "${buildParamPrefix}CLOUDCASA_VELERO",
            defaultValue: true,
            description: "Build and push assembled cloudcasa-velero image"
        ),
        booleanParam(
            name: "UPDATE_PLUGINS_INI_VELERO_TAG",
            defaultValue: true,
            description: "After a successful build, commit the new velero image tag to [velero] in plugins.ini"
        ),
        booleanParam(
            name: "PREPARE_REPO_FORCE_MASTER_SCOPE",
            defaultValue: false,
            description: "Run integration prepare-repo stage even when BRANCH_NAME is not a master-equivalent branch"
        ),
        booleanParam(
            name: "SKIP_PREPARE_DEPLOYMENT_REPO",
            defaultValue: false,
            description: "Skip the Prepare deployment stage"
        ),
        booleanParam(
            name: "PREPARE_REPO_DRY_RUN",
            defaultValue: false,
            description: "Do not commit/push deployment repo changes; show and archive diff only"
        ),
        string(
            name: "VELEROPLUGIN_IMAGE_NAME",
            defaultValue: "amds-veleroplugin",
            description: "Plugin image name used in plugins.ini for cloudcasa-velero"
        ),
        string(
            name: "VELEROPLUGIN_VERSION",
            defaultValue: "",
            description: "Optional explicit plugin version. If empty, use committed plugins.ini plugin version"
        ),
        string(
            name: "AWS_PLUGIN_IMAGE_OVERRIDE",
            defaultValue: "",
            description: "Optional full image override for [plugin:aws] in plugins.ini (for example, registry/repo:tag)"
        ),
        string(
            name: "GCP_PLUGIN_IMAGE_OVERRIDE",
            defaultValue: "",
            description: "Optional full image override for [plugin:gcp] in plugins.ini (for example, registry/repo:tag)"
        ),
        string(
            name: "AZURE_PLUGIN_IMAGE_OVERRIDE",
            defaultValue: "",
            description: "Optional full image override for [plugin:azure] in plugins.ini (for example, registry/repo:tag)"
        ),
        string(
            name: "KUBEVIRT_PLUGIN_IMAGE_OVERRIDE",
            defaultValue: "",
            description: "Optional full image override for [plugin:kubevirt] in plugins.ini (for example, registry/repo:tag)"
        ),
        string(
            name: "AMDS_PLUGIN_IMAGE_OVERRIDE",
            defaultValue: "",
            description: "Optional full image override for [plugin:amds] in plugins.ini (for example, registry/repo:tag)"
        )
    ])
])

node("cloudcasa-build") {
    stage("Checkout") {
        cleanWs()
        checkout scm
    }

    // Stop the build triggered by our own plugins.ini tag-bump commit (prevents a build loop).
    def lastCommitMsg = sh(script: "git log -1 --pretty=%s", returnStdout: true).trim()
    if (lastCommitMsg.startsWith("ci: bump velero image")) {
        echo "HEAD is the CI velero tag bump ('${lastCommitMsg}'); skipping this build."
        currentBuild.result = "NOT_BUILT"
        currentBuild.description = "Skipped: plugins.ini velero tag bump"
        return
    }

    def sourceBranch = env.BRANCH_NAME ?: "unknown"
    def masterEquivalentBranches = ["master", "v1.14.0.x", "v1.18.4.x", "jg-KUBEDR-7845"]
    def isMasterFlow = masterEquivalentBranches.contains(sourceBranch)
    def isProductionFlow = sourceBranch == "production"
    // production builds/pushes just like the master-equivalent branches, but tags
    // and deployment-repo targets differ (see branchTag and Prepare deployment repo below).
    def isReleaseFlow = isMasterFlow || isProductionFlow
    // Use "master"/"prod" as the tag segment for release branches so the tag stays
    // clean (e.g. 1.14.0-master.12, 1.14.0-prod.12) regardless of the actual branch name.
    def branchTag = isMasterFlow ? "master" : (isProductionFlow ? "prod" : sourceBranch.replaceAll('[^0-9A-Za-z-]', '-'))

    // Keep tags cloudcasa-like while preserving velero version context.
    def baseVersion = "1.18.4"
    def veleroTag = "v${baseVersion}-${branchTag}.${env.BUILD_NUMBER}"
    def cloudcasaVeleroTag = "${baseVersion}-${branchTag}.${env.BUILD_NUMBER}"

    def dockerRegistryInternal = env.DOCKER_REGISTRY_INTERNAL
    def dockerRegistryCredsInternal = env.DOCKER_REGISTRY_CREDENTIALS_INTERNAL
    def dockerPrefixInternal = "${dockerRegistryInternal}/catalogicsoftware"
    def dockerRegistryExternal = env.DOCKER_REGISTRY_EXTERNAL ?: ""
    def dockerRegistryCredsExternal = env.DOCKER_REGISTRY_CREDENTIALS_EXTERNAL ?: "docker.io-docker-registry"
    def dockerPrefixExternal = env.DOCKER_PREFIX_EXTERNAL ?: "catalogicsoftware"

    // Build on every branch; only master/production (isReleaseFlow) push to the
    // deployment repo below, gated separately via runPrepareRepo.
    def buildVelero = (params["${buildParamPrefix}VELERO"] ?: false)
    def buildCloudcasaVelero = (params["${buildParamPrefix}CLOUDCASA_VELERO"] ?: false)
    def runPrepareRepo = isReleaseFlow || (params.PREPARE_REPO_FORCE_MASTER_SCOPE ? params.PREPARE_REPO_FORCE_MASTER_SCOPE.toBoolean() : false)
    def prepareRepoDryRun = params.PREPARE_REPO_DRY_RUN ? params.PREPARE_REPO_DRY_RUN.toBoolean() : false

    stage("Build velero image") {
        if (buildVelero) {
            env.BUILDX_CONFIG = "${env.HOME}/.docker/buildx"
            docker.withRegistry("https://${dockerRegistryInternal}", dockerRegistryCredsInternal) {
                sh """
                    set -eu
                    make container \
                        REGISTRY=${dockerPrefixInternal} \
                        VERSION=${veleroTag} \
                        BUILD_OS=linux \
                        BUILD_ARCH=amd64,arm64 \
                        BUILD_OUTPUT_TYPE=registry \
                        IMAGE_TAGS='${dockerPrefixInternal}/velero:${veleroTag}'
                """
            }
        }
    }

    stage("Push velero image to external registry") {
        if (buildVelero) {
            env.BUILDX_CONFIG = "${env.HOME}/.docker/buildx"
            docker.withRegistry("https://${dockerRegistryInternal}", dockerRegistryCredsInternal) {
                docker.withRegistry(dockerRegistryExternal ? "https://${dockerRegistryExternal}" : "", dockerRegistryCredsExternal) {
                    sh """
                        set -eu
                        docker buildx imagetools create \
                            --tag ${dockerPrefixExternal}/velero:${veleroTag} \
                            ${dockerPrefixInternal}/velero:${veleroTag}
                    """
                }
            }
        }
    }

    stage("Build cloudcasa-velero image") {
        if (buildCloudcasaVelero) {
            env.BUILDX_CONFIG = "${env.HOME}/.docker/buildx"

            def pluginImageName = (params.VELEROPLUGIN_IMAGE_NAME ?: "amds-veleroplugin").trim()
            def pluginVersion = (params.VELEROPLUGIN_VERSION ?: "").trim()
            def pluginImageOverrides = [
                aws     : (params.AWS_PLUGIN_IMAGE_OVERRIDE ?: "").trim(),
                gcp     : (params.GCP_PLUGIN_IMAGE_OVERRIDE ?: "").trim(),
                azure   : (params.AZURE_PLUGIN_IMAGE_OVERRIDE ?: "").trim(),
                kubevirt: (params.KUBEVIRT_PLUGIN_IMAGE_OVERRIDE ?: "").trim(),
                amds    : (params.AMDS_PLUGIN_IMAGE_OVERRIDE ?: "").trim()
            ]

            def setPluginImageInConfig = { String pluginSection, String pluginImage ->
                withEnv([
                    "PLUGIN_SECTION=${pluginSection}",
                    "PLUGIN_IMAGE=${pluginImage}"
                ]) {
                    sh '''
                        set -eu
                        awk '
                            BEGIN {
                                in_plugin = 0
                                plugin_header = "[plugin:" ENVIRON["PLUGIN_SECTION"] "]"
                            }
                            $0 == plugin_header { in_plugin = 1; print; next }
                            substr($0, 1, 1) == "[" { in_plugin = 0 }
                            in_plugin && $0 ~ /^image[[:space:]]*=/ {
                                print "image = " ENVIRON["PLUGIN_IMAGE"]
                                next
                            }
                            { print }
                        ' plugins.ini > plugins.ini.tmp
                        mv plugins.ini.tmp plugins.ini
                    '''
                }
            }

            // Keep the config deterministic: use this build's velero image tag.
            def veleroBaseTagForCloudcasa = veleroTag
            withEnv([
                "VELERO_BASE_IMAGE=${dockerPrefixInternal}/velero:${veleroBaseTagForCloudcasa}"
            ]) {
                sh '''
                    set -eu
                    awk '
                        BEGIN { in_velero = 0 }
                        $0 == "[velero]" { in_velero = 1; print; next }
                        substr($0, 1, 1) == "[" { in_velero = 0 }
                        in_velero && $0 ~ /^image[[:space:]]*=/ {
                            print "image = " ENVIRON["VELERO_BASE_IMAGE"]
                            next
                        }
                        { print }
                    ' plugins.ini > plugins.ini.tmp
                    mv plugins.ini.tmp plugins.ini
                '''
            }

            if (pluginVersion && !pluginImageOverrides.amds) {
                withEnv([
                    "AMDS_PLUGIN_IMAGE=catalogicsoftware/${pluginImageName}:${pluginVersion}"
                ]) {
                    sh '''
                        set -eu
                        awk '
                            BEGIN { in_amds = 0 }
                            $0 == "[plugin:amds]" { in_amds = 1; print; next }
                            substr($0, 1, 1) == "[" { in_amds = 0 }
                            in_amds && $0 ~ /^image[[:space:]]*=/ {
                                print "image = " ENVIRON["AMDS_PLUGIN_IMAGE"]
                                next
                            }
                            { print }
                        ' plugins.ini > plugins.ini.tmp
                        mv plugins.ini.tmp plugins.ini
                    '''
                }
            }

            pluginImageOverrides.each { pluginSection, pluginImage ->
                if (pluginImage) {
                    setPluginImageInConfig(pluginSection as String, pluginImage as String)
                }
            }

            docker.withRegistry("https://${dockerRegistryInternal}", dockerRegistryCredsInternal) {
                sh """
                    set -eu
                    bash build-plugins.sh \
                        --config plugins.ini \
                        ${dockerPrefixInternal}/cloudcasa-velero:${cloudcasaVeleroTag} \
                        --platform linux/amd64,linux/arm64 \
                        --push
                """
            }

            archiveArtifacts artifacts: "plugins.ini", onlyIfSuccessful: true
        }
    }

    stage("Push cloudcasa-velero image to external registry") {
        if (buildCloudcasaVelero) {
            env.BUILDX_CONFIG = "${env.HOME}/.docker/buildx"
            docker.withRegistry("https://${dockerRegistryInternal}", dockerRegistryCredsInternal) {
                docker.withRegistry(dockerRegistryExternal ? "https://${dockerRegistryExternal}" : "", dockerRegistryCredsExternal) {
                    sh """
                        set -eu
                        docker buildx imagetools create \
                            --tag ${dockerPrefixExternal}/cloudcasa-velero:${cloudcasaVeleroTag} \
                            ${dockerPrefixInternal}/cloudcasa-velero:${cloudcasaVeleroTag}
                    """
                }
            }
        }
    }

    stage("Push cloudcasa-velero to ACR") {
        if (buildCloudcasaVelero) {
            env.BUILDX_CONFIG = "${env.HOME}/.docker/buildx"
            def acrRegistry = env.CLOUDCASA_ACR_REGISTRY ?: "cloudcasaAgent.azurecr.io"
            def acrCredentialsId = env.CLOUDCASA_ACR_CREDENTIALS_ID ?: "cloudcasaAgent-azurecr"
            withCredentials([usernamePassword(
                credentialsId: acrCredentialsId,
                usernameVariable: 'ACR_USER',
                passwordVariable: 'ACR_PASS'
            )]) {
                docker.withRegistry("https://${dockerRegistryInternal}", dockerRegistryCredsInternal) {
                    sh """
                        set -eu
                        docker login -u \$ACR_USER -p \$ACR_PASS ${acrRegistry}
                        docker buildx imagetools create \
                            --tag ${acrRegistry}/catalogicsoftware/cloudcasa-velero:${cloudcasaVeleroTag} \
                            ${dockerPrefixInternal}/cloudcasa-velero:${cloudcasaVeleroTag}
                    """
                }
            }
        }
    }

    stage("Update plugins.ini velero tag in repo") {
        // Only on these branches, so test branches never commit
        def pluginsIniUpdateBranches = ["v1.18.4.x"]
        if (buildCloudcasaVelero && (params.UPDATE_PLUGINS_INI_VELERO_TAG ?: false) && pluginsIniUpdateBranches.contains(sourceBranch)) {
            withCredentials([usernamePassword(
                credentialsId: 'github-access-token',
                usernameVariable: 'GIT_USER',
                passwordVariable: 'GIT_PASS'
            )]) {
                def originUrl = sh(script: "git config --get remote.origin.url", returnStdout: true).trim()
                def repoHost = originUrl.replaceFirst('https://', '').replaceFirst('^[^@]*@', '')

                sh """
                    git config user.name 'cloudcasabot'
                    git config user.email 'cloudcasabot@catalogicsoftware.com'
                """

                // Start from the latest remote branch (plugin jobs also commit here), discarding
                // the workspace-only plugins.ini edits made during the build.
                sh """
                    set -eu
                    git fetch https://\${GIT_USER}:\${GIT_PASS}@${repoHost} ${sourceBranch}
                    git checkout -f -B ${sourceBranch} FETCH_HEAD
                """

                // Tag-only change inside [velero]; keeps the existing registry/org prefix.
                withEnv(["NEW_TAG=${veleroTag}"]) {
                    sh '''
                        set -eu
                        sed -i -E "/^\\[velero\\]/,/^\\[/ s|^(image[[:space:]]*=[[:space:]]*[^[:space:]]*velero:)[^[:space:]]+|\\1${NEW_TAG}|" plugins.ini
                        grep -qF "velero:${NEW_TAG}" plugins.ini || {
                            echo "Failed to update velero tag in plugins.ini" >&2
                            exit 1
                        }
                    '''
                }

                sh """
                    set -eu
                    if ! git diff --quiet -- plugins.ini; then
                        git add plugins.ini
                        git commit -m "ci: bump velero image to ${veleroTag} [skip ci] [${env.JOB_NAME} #${env.BUILD_NUMBER}]"
                        git push https://\${GIT_USER}:\${GIT_PASS}@${repoHost} HEAD:${sourceBranch}
                    else
                        echo "plugins.ini [velero] already at ${veleroTag}"
                    fi
                """
            }
        } else {
            echo "Skipping plugins.ini velero tag update"
        }
    }

    // Point the selfhosted helmchart at the cloudcasa-velero image just pushed to the agent ACR
    // (images.agentRegistry). Only the top-level images.veleroTag; images.openshift.veleroTag
    // tracks the Red Hat certified production image and is managed separately.
    stage("Update selfhosted helmchart veleroTag") {
        if (buildCloudcasaVelero && isMasterFlow) {
            def helmchartRepoBranch = "main"
            withCredentials([
                usernamePassword(
                    credentialsId: 'github-access-token',
                    usernameVariable: 'GIT_USER',
                    passwordVariable: 'GIT_PASS'
                )
            ]) {
                dir('cloudcasa-server-helmchart') {
                    git url: 'https://github.com/catalogicsoftware/cloudcasa-server-helmchart.git',
                        branch: helmchartRepoBranch,
                        credentialsId: 'github-access-token'

                    sh """
                        git config user.name 'cloudcasabot'
                        git config user.email 'cloudcasabot@catalogicsoftware.com'
                    """

                    sh """
                        set -eu
                        sed -i -E "s/^  veleroTag:.*/  veleroTag: ${cloudcasaVeleroTag}/" values.yaml

                        if [ -n "\$(git status --porcelain)" ]; then
                            if [ "${prepareRepoDryRun}" = "true" ]; then
                                echo "DRY RUN: helmchart changes detected; skipping commit/push"
                                git diff --patch
                            else
                                git add values.yaml
                                git commit -m "ci: update velero selfhosted tag to ${cloudcasaVeleroTag} [${env.JOB_NAME} #${env.BUILD_NUMBER}]"
                                git pull --rebase https://\${GIT_USER}:\${GIT_PASS}@github.com/catalogicsoftware/cloudcasa-server-helmchart.git ${helmchartRepoBranch}
                                git push https://\${GIT_USER}:\${GIT_PASS}@github.com/catalogicsoftware/cloudcasa-server-helmchart.git ${helmchartRepoBranch}
                            fi
                        else
                            echo "No helmchart veleroTag changes for cloudcasa-velero"
                        fi
                    """
                }
            }
        }
    }

    stage("Red Hat certification (cloudcasa-velero)") {
        if (buildCloudcasaVelero && isProductionFlow) {
            def sourceImage = "${dockerPrefixInternal}/cloudcasa-velero:${cloudcasaVeleroTag}"

            env.BUILDX_CONFIG = "${env.HOME}/.docker/buildx"
            withCredentials([
                usernamePassword(
                    credentialsId: dockerRegistryCredsInternal,
                    usernameVariable: 'INTERNAL_USER',
                    passwordVariable: 'INTERNAL_PASS'
                ),
                usernamePassword(
                    credentialsId: 'quay-redhat-isv-cloudcasa-velero-robot',
                    usernameVariable: 'QUAY_USER',
                    passwordVariable: 'QUAY_PASS'
                ),
                string(credentialsId: 'pyxis-api-token', variable: 'PFLT_PYXIS_API_TOKEN')
            ]) {
                // Quay ISV robot accounts are named "redhat-isv-containers+<component-id>-robot";
                // pull the component ID from there instead of duplicating it in a constant/env var.
                def ocpCertComponentId = (env.QUAY_USER =~ /^redhat-isv-containers\+(.+)-robot$/)[0][1]
                def quayCertImage = "quay.io/redhat-isv-containers/${ocpCertComponentId}:${cloudcasaVeleroTag}"

                // Plain `docker login` calls (not docker.withRegistry, which scopes logins to its
                // own isolated config and never touches $HOME/.docker/config.json) so both this
                // registry auth and the preflight run below share the same real docker config.
                sh """
                    set -eu
                    echo \$INTERNAL_PASS | docker login -u \$INTERNAL_USER --password-stdin ${dockerRegistryInternal}
                    echo \$QUAY_PASS | docker login -u \$QUAY_USER --password-stdin quay.io
                    docker buildx imagetools create --tag ${quayCertImage} ${sourceImage}
                """

                sh """
                    set -eu
                    docker run --rm \
                        -v \${HOME}/.docker:/root/.docker:ro \
                        -e PFLT_PYXIS_API_TOKEN=\$PFLT_PYXIS_API_TOKEN \
                        quay.io/opdev/preflight:stable check container \
                        --certification-component-id ${ocpCertComponentId} \
                        --loglevel debug \
                        -d /root/.docker/config.json \
                        ${quayCertImage} \
                        --submit
                """
            }
        }
    }

    stage("Prepare deployment repo") {
        //if (buildCloudcasaVelero && runPrepareRepo) {
        if (buildCloudcasaVelero && runPrepareRepo && !(params.SKIP_PREPARE_DEPLOYMENT_REPO ?: false)) {
            withCredentials([
                usernamePassword(
                    credentialsId: 'github-access-token',
                    usernameVariable: 'GIT_USER',
                    passwordVariable: 'GIT_PASS'
                )
            ]) {
                def deploymentRepoUrl = env.CLOUDCASA_DEPLOYMENT_REPO_URL
                def deploymentRepoHost = deploymentRepoUrl.replaceFirst('https://', '')
                dir('cloudcasa-deployment') {
                    git url: deploymentRepoUrl,
                        branch: 'master',
                        credentialsId: 'github-access-token'

                    sh """
                        git config user.name 'cloudcasabot'
                        git config user.email 'cloudcasabot@catalogicsoftware.com'
                    """

                    // Patch cloudcasa-velero image tag in the specific files that Concourse
                    // k8s-prepare-repo.sh targets. Match the full ACR-prefixed reference so
                    // other registries (e.g. OpenShift) are not accidentally patched.
                    // production has its own overlay (archimedes/prod/*) and must not touch
                    // the base/local/integration files that the v1.14.0.x flow updates.
                    def acrImage = "cloudcasaAgent.azurecr.io/catalogicsoftware/cloudcasa-velero"
                    def patchTargets = isProductionFlow ?
                        [
                            [file: "archimedes/prod/kas/deployment_kas_image_spec.yaml", pattern: acrImage],
                            [file: "archimedes/prod/global-cm.yaml",                     pattern: "velero.imageRef=${acrImage}"]
                        ] :
                        [
                            [file: "archimedes/base/kas/deployment.yaml",                       pattern: acrImage],
                            [file: "archimedes/local/global-cm.yaml",                           pattern: "velero.imageRef=${acrImage}"],
                            [file: "archimedes/integration/kas/deployment_kas_image_spec.yaml", pattern: acrImage],
                            [file: "archimedes/integration/global-cm.yaml",                      pattern: "velero.imageRef=${acrImage}"]
                        ]

                    withEnv(["TAG=${cloudcasaVeleroTag}"]) {
                        patchTargets.each { target ->
                            withEnv([
                                "PATCH_FILE=${target.file}",
                                "PATCH_PATTERN=${target.pattern}"
                            ]) {
                                sh '''
                                    set -eu
                                    [ -f "$PATCH_FILE" ] || exit 0
                                    sed -i "s|${PATCH_PATTERN}:.*|${PATCH_PATTERN}:${TAG}|g" "$PATCH_FILE"
                                '''
                            }
                        }
                    }

                    sh """
                        set -eu
                        if [ -n "\$(git status --porcelain)" ]; then
                            if [ "${prepareRepoDryRun}" = "true" ]; then
                                echo "DRY RUN: deployment repo changes detected; skipping commit/push"
                                git status --short
                                git diff --patch > deployment-repo-dry-run.patch
                            else
                                git add -A
                                git commit -m "ci: update cloudcasa-velero to ${cloudcasaVeleroTag} from ${env.JOB_NAME} #${env.BUILD_NUMBER}"
                                git push https://${GIT_USER}:${GIT_PASS}@${deploymentRepoHost} master
                            fi
                        else
                            echo "No deployment repo changes for cloudcasa-velero"
                        fi
                    """

                    if (prepareRepoDryRun) {
                        archiveArtifacts artifacts: 'deployment-repo-dry-run.patch', allowEmptyArchive: true, onlyIfSuccessful: true
                    }
                }
            }
        }
    }
}
