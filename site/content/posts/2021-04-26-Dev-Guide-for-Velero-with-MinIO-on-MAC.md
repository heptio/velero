---
title: Developer's Guide for Velero with MinIO. (MacOS Version)
slug: Dev-Guide-for-Velero-with-MinIO-on-MAC
# image: https://placehold.it/200x200
excerpt: One of the things that many find frustrating when contributing to a new project is, struggling to correctly build a development environment. With that in mind, here's a handy guide, which walks through the required tools and configurations that are needed to build Developer's environment for Velero with MinIO on MAC OS.
author_name: Dave Mazur, Rafael Brito, Pradeep Jigalur, Frankie Gold
# author_avatar: https://placehold.it/64x64
categories: ['velero']
# Tag should match author to drive author pages
tags: ['Velero Team']
---
One of the things that many find frustrating when contributing to a new project is, correctly building a development environment. With that in mind, here's a handy guide, which walks through the required tools and configurations that are needed to build developer's environment for Velero with MinIO on MAC OS.  

**Note:** The following are suggested tools and can be replaced per your needs.

### Prerequisite tools:
- Docker Desktop
- A DockerHub account
- kubectl
- Kind
- MinIO as an object storage service.
- A GitHub account
- Go

## Installing Prerequisite Tools
Follow the below instructions to install the prerequisite tools.

### 1. Configure Docker Desktop
Refer to the following official Docker documentation page to install Docker Desktop  
https://docs.docker.com/docker-for-mac/install/
- Verify Docker is installed correctly by checking Docker version:    
    ```bash
    $ docker --version
    Docker version 19.03.12, build 48a66213fe
    ```

- Enable experimental features on Docker, as this is required to build a Velero container image later 
    - Go into "Preferences-> Command Line" and Enable Experimental features

  ![enabledockerexperimentalfeature](/img/posts/enabledockerexperimentalfeature.png)   
    - Select "Apply & Restart" docker  

### 2. Configure Dockerhub image registry account  
- Create a Dockerhub account by following:  
  https://docs.docker.com/docker-hub/#step-1-sign-up-for-a-docker-account

- After successful account creation, login from the terminal  
    ```bash
    $ docker login
    Authenticating with existing credentials...
    Login Succeeded
    ```  
  
### 3. Install kubectl
- Refer to the below page to install the kubectl command-line utility   
  https://kubernetes.io/docs/tasks/tools/install-kubectl-macos  

- Verify `kubectl` is installed correctly by checking `kubectl` version 
    ```bash
    $ kubectl version --client --short
    Client Version: v1.18.2
    ```  
  
### 4. Install MinIO as an Object storage service
  MinIO using a local directory can be used as an object storage for Velero backup and restore  
  More information on MinIO can be found at https://min.io/

- Install MinIO using brew  
    ```bash
    $ brew install minio/stable/minio
    ==> Tapping minio/stable
    Cloning into '/usr/local/Homebrew/Library/Taps/minio/homebrew-stable'...
    (...)
    ==> Installing minio from minio/stable
    ==> Downloading https://dl.minio.io/server/minio/release/darwin-amd64/minio.RELEASE.2021-04-06T23-11-00Z
    ######################################################################## 100.0%
    ==> Download complete!
    (......)
    ```

- Create a directory, which will be used as persistent Storage for MinIO  
    ```bash
    $ cd ~
    $ mkdir velero-storage
    $ ls
    velero-storage
    ```

- Start the MinIO Server by setting the environment variables  
    ```bash
    $ export MINIO_ACCESS_KEY=minio
    $ export MINIO_SECRET_KEY=miniostorage
    $ minio server ~/projects/velero-storage
    Attempting encryption of all config, IAM users and policies on MinIO backend
    Endpoint: http://192.168.29.155:9000  http://10.104.68.132:9000  http://127.0.0.1:9000
    RootUser: minio
    RootPass: miniostorage
    (...)
    IAM initialization complete
    ```

- Login to the MinIO UI, by using the endpoint URL, rootuser(access key), and rootpass(secret key)(These details are logged on to the console by the previous command)  
  ![miniologinpage](/img/posts/miniologinpage.png)  
  
- Install the MinIO command line client
    ```bash
    $ brew install minio/stable/mc
    ==> Installing mc from minio/stable
    ==> Downloading https://dl.minio.io/client/mc/release/darwin-amd64/mc.RELEASE.2021-03-23T05-46-11Z
    ######################################################################## 100.0%
    /usr/local/Cellar/mc/RELEASE.2021-03-23T05-46-11Z_1: 3 files, 21.2MB, built in 9 seconds
    ```

- Add MinIO `rootuser` and `rootpass` values to the client alias for convenience    
    ```bash
    $ mc alias set myminio http://localhost:9000 minio miniostorage
    Added `myminio` successfully
    ```

- Create a new storage bucket on MinIO Storage  
    ```bash
    $ mc mb myminio/velerobackup
    Bucket created successfully `myminio/velerobackup`.
    ```

- Create a MinIO credentials file  
    ```bash
    $ cat ~/.credentials-minio
    [default]
    aws_access_key_id = minio
    aws_secret_access_key = miniostorage
    ```

- Set the following MinIO objects as environment variables
    ```bash
    $ export BUCKET=velerobackup
    $ export REGION=minio,s3ForcePathStyle="true",s3Url=http://192.168.29.155:9000
    $ export SECRETFILE=~/.credentials-minio
    $ export IMAGE=velero/velero:latest
    ```

### 5. Install the Velero Client
- Install Velero by following the instructions at  
  https://velero.io/docs/v1.6/basic-install/#option-2-github-release

- Ensure Velero is installed properly by running the below command  
  ```bash
  $ velero version --client-only
  Client:
      Version: v1.5.1
      Git commit: 87d86a45a6ca66c6c942c7c7f08352e26809426c
  ``` 

### 6. Install KinD (Kubernetes in Docker) and create a Kind cluster
- More information on `kind` can be found at https://kind.sigs.Kubernetes.io/

- Install kind using brew  
  ```bash
  $ brew install kind
  ==> Downloading https://ghcr.io/v2/homebrew/core/kind/manifests/0.10.0
  (...)
  ==> Summary
  /usr/local/Cellar/kind/0.10.0: 8 files, 9.2MB
  Removing: ~/Library/Caches/Homebrew/kind--... (4.7MB)
  ```
  
- Ensure `kind` is installed properly by running the below command  
  ```bash
  $ kind version
  kind v0.10.0 go1.15.7 darwin/amd64
  ```

- Create a Kubernetes cluster using Kind  
  ```bash
  $ kind create cluster --name=dev-cluster
  Creating cluster "dev-cluster" ...
  ✓ Ensuring node image (kindest/node:v1.20.2)
  ✓ Preparing nodes
  ✓ Writing configuration
  ✓ Starting control-plane ️
  ✓ Installing CNI
  ✓ Installing StorageClass
  Set kubectl context to "kind-dev-cluster"
  You can now use your cluster with:

  kubectl cluster-info --context kind-dev-cluster

  Thanks for using kind!
  ```

- Make sure the cluster exists  
  ```bash
  $ kind get clusters
  dev-cluster
  kind
  ```

### 7. Create Sample Kubernetes Objects that Velero will Back up and Restore for test
- Create a Kubernetes namespace  
  ```bash
  $ kubectl create namespace dev
  namespace/dev created
  ```

- Create a Kubernetes deployment for `nginx` pods  
  ```bash
  $ kubectl create  deployment nginx  --image=nginx --namespace dev
  deployment.apps/nginx created
  ```  

- Ensure the deployment is created and is running  
  ```bash
  $ kubectl get all -n dev
  NAME                         READY   STATUS    RESTARTS   AGE
  pod/nginx-6799fc88d8-hdrfw   1/1     Running   0          4m9s

  NAME                    READY   UP-TO-DATE   AVAILABLE   AGE
  deployment.apps/nginx   1/1     1            1           4m9s

  NAME                               DESIRED   CURRENT   READY   AGE
  replicaset.apps/nginx-6799fc88d8   1         1         1       4m9s
  ```

## Let's get started with Velero  

### Test Kubernetes Cluster Backup and Restore using Velero  

### 1. Install the Velero backup controller in the cluster  
- Run the Velero install command  
  ```bash
  $ velero install --provider aws \
  --plugins velero/velero-plugin-for-aws:v1.0.0 \
  --bucket $BUCKET \
  --backup-location-config region=$REGION  \
  --secret-file $SECRETFILE \
  --image $IMAGE --wait
  CustomResourceDefinition/backups.velero.io: attempting to create resource
  CustomResourceDefinition/backups.velero.io: created
  (...)
  Deployment/velero: attempting to create resource
  Deployment/velero: created
  Waiting for Velero deployment to be ready.
  Velero is installed! Use 'kubectl logs deployment/velero -n velero' to view the status.
  ```

- Ensure `velero` deployment is running inside the `velero` namespace  
  ```bash
  $ kubectl get all -n velero  
  NAME                         READY   STATUS    RESTARTS   AGE
  pod/velero-8d7c7b978-8ldrt   1/1     Running   0          4m40s

  NAME                     READY   UP-TO-DATE   AVAILABLE   AGE
  deployment.apps/velero   1/1     1            1           4m40s

  NAME                               DESIRED   CURRENT   READY   AGE
  replicaset.apps/velero-8d7c7b978   1         1         1       4m40s
  ```

### 2. Back up Kubernetes Clusters on MinIO using Velero

- Start the Kubernetes cluster backup using Velero. Velero will save the backup file on the installed object storage service
  ```bash
  $ velero backup create backup1 --wait
  Backup request "backup1" submitted successfully.
  Waiting for backup to complete. You may safely press ctrl-c to stop waiting - your backup will continue in the background.
  (...)
  Backup completed with status: Completed.
  You may check for more information using the commands `velero backup describe backup1` and `velero backup logs backup1`.
  ```

- Confirm backup is available by logging into  MinIO UI    

### 3. Delete Kubernetes Cluster
- Delete the Kubernetes cluster created earlier  
  ```bash
  $ kind delete cluster --name=dev-cluster
  Deleting cluster "dev-cluster" ...
  ```

- Ensure cluster is deleted  
  ```bash
  $ kind get clusters
  kind
  ```

The Kubernetes cluster with name `dev-cluster` will be deleted.

### 4. Restore cluster from the Velero Backup stored in MinIO storage
- Create an empty Kubernetes cluster with the name `dev-cluster`  
  ```bash
  $ kind create cluster --name=dev-cluster
  Creating cluster "dev-cluster" ...
  ✓ Ensuring node image (kindest/node:v1.20.2)
  ✓ Preparing nodes
  ✓ Writing configuration
  ✓ Starting control-plane
  ✓ Installing CNI
  ✓ Installing StorageClass
  Set kubectl context to "kind-dev-cluster"
  You can now use your cluster with:

  kubectl cluster-info --context kind-dev-cluster

  Have a nice day!
  ```

- Run the Velero install command  
  ```bash
  $ velero install --provider aws \
  --plugins velero/velero-plugin-for-aws:v1.0.0 \
  --bucket $BUCKET \
  --backup-location-config region=$REGION  \
  --secret-file $SECRETFILE \
  --image $IMAGE --wait
  CustomResourceDefinition/backups.velero.io: attempting to create resource
  (...)
  Deployment/velero: attempting to create resource
  Deployment/velero: created
  Waiting for Velero deployment to be ready.
  Velero is installed!  Use 'kubectl logs deployment/velero -n velero' to view the status.
  ```

- Ensure `velero` deployment is running inside the `velero` namespace  
  ```bash
  $ kubectl get all -n velero
  NAME                         READY   STATUS    RESTARTS   AGE
  pod/velero-8d7c7b978-vz2tp   1/1     Running   0          2m

  NAME                     READY   UP-TO-DATE   AVAILABLE   AGE
  deployment.apps/velero   1/1     1            1           2m

  NAME                               DESIRED   CURRENT   READY   AGE
  replicaset.apps/velero-8d7c7b978   1         1         1       2m
  ```

- Restore the cluster from backup `backup1`  
  ```bash
  $ velero restore create restore1 --from-backup backup1 --wait
  Restore request "restore1" submitted successfully.
  Waiting for restore to complete. You may safely press ctrl-c to stop waiting - your restore will continue in the background.
  ...............................
  Restore completed with status: Completed.
  You may check for more information using the commands `velero restore describe restore1` and `velero restore logs restore1`.
  ```

- Ensure all Kubernetes objects are created and are running  
  ```bash
  $ kubectl get all -n dev
  NAME                         READY   STATUS    RESTARTS   AGE
  pod/nginx-6799fc88d8-hdrfw   1/1     Running   0          2m28s

  NAME                    READY   UP-TO-DATE   AVAILABLE   AGE
  deployment.apps/nginx   1/1     1            1           2m12s

  NAME                               DESIRED   CURRENT   READY   AGE
  replicaset.apps/nginx-6799fc88d8   1         1         1       2m28s
  ```

## Now, Let's GO Dig into the Development  
- Install Go following the instructions at  https://golang.org/doc/install

- Clone the Velero source code  
  ```bash
  $ mkdir -p ~/go/src/github.com/vmware-tanzu/
  $ cd ~/go/src/github.com/vmware-tanzu/
  $ git clone https://github.com/vmware-tanzu/velero.git
  Cloning into 'velero'...
  remote: Enumerating objects: 141, done.
  remote: Counting objects: 100% (141/141), done.
  remote: Compressing objects: 100% (97/97), done.
  remote: Total 35112 (delta 64), reused 71 (delta 32), pack-reused 34971
  Receiving objects: 100% (35112/35112), 31.68 MiB | 7.99 MiB/s, done.
  Resolving deltas: 100% (23053/23053), done.
  Updating files: 100% (2069/2069), done.
  ```

- Perform changes on the Velero source code  

- Compile and build the Velero binary  
  ```bash
  $ cd velero
  $ make local
    make: realpath: Command not found
    GOOS=darwin \
    GOARCH=amd64 \
    VERSION=main \
    PKG=github.com/vmware-tanzu/velero \
    BIN=velero \
    GIT_SHA=9f24587cef930ddb5679379a0d726fef3354af42 \
    GIT_TREE_STATE=clean \
    OUTPUT_DIR=$(pwd)/_output/bin/darwin/amd64 \
    ./hack/build.sh
  ```
  This command creates a Velero executable in the directory, **VELERO_ROOT/_output/bin/darwin/amd64/**

- Test Velero changes with the following steps  
  - Replace the default `velero` executable, with a newly build executable  
    ```bash
    $ mv ~/go/src/github.com/vmware-tanzu/velero/_output/bin/darwin/amd64/velero /usr/local/bin/velero
    ```  
    
  - Run Velero executable
    ```bash
    $ velero version
    Client:
    Version: main
    Git commit: 9f24587cef930ddb5679379a0d726fef3354af42
    Server:
    Version: v1.5.4
    ```

## Test the Changes
### 1. Build a `velero` Docker container image with the changes

- Build `velero` Docker container image
  ```bash
  $ make container
  [+] Building 374.9s (14/14) FINISHED
  => [internal] load build definition from Dockerfile                                                                              0.1s
  => => transferring dockerfile: 1.79kB                                                                                            0.0s
  => [internal] load .dockerignore                                                                                                 0.1s
   (...)
  => => writing image sha256:8bdafe53593892fffb5fc297f8c0613bb962fd70498732bb9695f32b140c56ec                                      0.0s
  => => naming to docker.io/velero/velero:main                                                                                     0.0s
  container: velero/velero:main
  ```

- Make sure the image has been built
  ```bash
  $ docker image ls
  REPOSITORY          TAG                 IMAGE ID            CREATED             SIZE
  velero/velero       main                8bdafe535938        13 minutes ago      168MB
  ```

### 2. Push the new `velero` image to your DockerHub registry
- Go into DockerHub and create a repository for the image  
  ![createrepoindockerhub](/img/posts/createrepoindockerhub.png)

- Create a tag for the image  
  ```bash
  $ docker tag COMMIT_ID DOCUKERHUB_ID/velero:v1
  ```

- Push the image to the DockerHub  
  ```bash
  $ docker push DockerHubId/velero
  The push refers to repository [docker.io/DockerHubId/velero]
  5dc34fa4b2c4: Pushed
  0d810434814b: Pushed
  346be19f13b0: Mounted from library/ubuntu
  (...)
  v1: digest: sha256:d3d39551f3759560db6f0d0bfb02882b42ec7c859f35f30845850bc923c1678b size: 1366
  ```

- Go into DockerHub and check that the tag has been pushed  
  ![imageindockerhub](/img/posts/imageindockerhub.png)

### 3. Run the `velero` install using the latest `velero` image which has your changes  
- Install the latest `velero` on the cluster  
  ```bash
  $ export VERSION=v1
  $ export IMAGE=hub.docker.com/DockerHubId/velero:$VERSION
  $ velero install --provider aws \
  --plugins velero/velero-plugin-for-aws:v1.0.0 \
  --bucket $BUCKET \
  --backup-location-config region=$REGION  \
  --secret-file $SECRETFILE \
  --image $IMAGE --wait
  ```  

#### Hopefully, this gets you started! Enjoy Velero and thank you for your contribution!