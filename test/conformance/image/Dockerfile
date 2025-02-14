# Copyright 2018 The Kubernetes Authors.
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

ARG BASEIMAGE
ARG RUNNERIMAGE

FROM ${BASEIMAGE} as debbase

FROM ${RUNNERIMAGE}

# This is a dependency for `kubectl diff` tests
COPY --from=debbase /usr/bin/diff /usr/local/bin/
COPY --from=debbase /lib/x86_64-linux-gnu/libc.so.6 /lib/x86_64-linux-gnu
COPY --from=debbase /lib/x86_64-linux-gnu/libpthread.so.0 /lib/x86_64-linux-gnu
COPY --from=debbase /lib64/ld-linux-x86-64.so.2 /lib64

COPY cluster /kubernetes/cluster
COPY ginkgo /usr/local/bin/
COPY e2e.test /usr/local/bin/
COPY kubectl /usr/local/bin/
COPY gorunner /usr/local/bin/kubeconformance

# Legacy executables -- deprecated
COPY gorunner /run_e2e.sh
COPY gorunner /gorunner

ENV E2E_FOCUS="\[Conformance\]"
ENV E2E_SKIP=""
ENV E2E_PROVIDER="local"
ENV E2E_PARALLEL="1"
ENV E2E_VERBOSITY="4"
ENV RESULTS_DIR="/tmp/results"
ENV KUBECONFIG=""

ENTRYPOINT [ "kubeconformance" ]
