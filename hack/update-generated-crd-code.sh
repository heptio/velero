#!/bin/bash
#
# Copyright the Velero contributors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

# this script expects to be run from the root of the Velero repo.

if [[ -z "${GOPATH}" ]]; then
  GOPATH=~/go
fi

if [[ ! -d "${GOPATH}/src/k8s.io/code-generator" ]]; then
  echo "k8s.io/code-generator missing from GOPATH"
  exit 1
fi

if ! command -v controller-gen > /dev/null; then
  echo "controller-gen is missing"
  exit 1
fi

${GOPATH}/src/k8s.io/code-generator/generate-groups.sh \
  all \
  github.com/vmware-tanzu/velero/pkg/generated \
  github.com/vmware-tanzu/velero/pkg/apis \
  "velero:v1" \
  --go-header-file ./hack/boilerplate.go.txt \
  --output-base ../../.. \
  $@

# Generate both apiextensions.k8s.io/v1beta1 and apiextensions.k8s.io/v1
for version in v1beta1 v1
do
  # Generate manifests e.g. CRD, RBAC etc.
  controller-gen \
    crd:crdVersions=$version,preserveUnknownFields=false,trivialVersions=true \
    paths=./pkg/apis/velero/v1/... \
    paths=./pkg/controller/... \
    output:crd:artifacts:config=config/crd/$version/bases

  # this is a super hacky workaround for https://github.com/kubernetes/kubernetes/issues/91395
  # which a result of fixing the validation on CRD objects. The validation ensures the fields that are list map keys, are either marked
  # as required or have default values to ensure merging of list map items work as expected.
  # With "containerPort" and "protocol" being considered as x-kubernetes-list-map-keys in the container ports, and "protocol" was not
  # a required field, the CRD would fail validation with errors similar to the one reported in https://github.com/kubernetes/kubernetes/issues/91395.
  # once controller-gen (above) is able to generate CRDs with `protocol` as a required field, this hack can be removed.
  kubectl patch -f config/crd/$version/bases/velero.io_restores.yaml -p "$(cat hack/restore-crd-patch-$version.json)" --type=json --local=true -o yaml > /tmp/velero.io_restores-yaml.patched
  mv /tmp/velero.io_restores-yaml.patched config/crd/$version/bases/velero.io_restores.yaml

  go generate ./config/crd/$version/crds
done
