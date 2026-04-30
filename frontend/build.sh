#
# Copyright 2025 coze-dev Authors
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
#

# --platform 按需修改，常见：linux/amd64（多数服务器）、linux/arm64（Apple Silicon 本机跑容器）
DOCKER_BUILDKIT=1 docker build --platform linux/amd64 -f frontend/Dockerfile -t coze-studio-frontend:latest .
TAG=${1:-hjaliyun20260429-1}
IMAGE=crpi-m48kvlo3g8s4dcsl.cn-shanghai.personal.cr.aliyuncs.com/ai-education-studio/coze-studio-web:${TAG}
docker tag coze-studio-frontend:latest ${IMAGE}
docker push ${IMAGE}
