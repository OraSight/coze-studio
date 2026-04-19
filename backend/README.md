# Coze Backend API

## 开发环境要求

- Go `1.24+`
- Docker / Docker Compose
- 建议在仓库根目录执行命令：`/Users/judith/workspace/other/openhydra/coze-studio`

## 方式一：本地开发模式（推荐）

后端进程在本机运行，MySQL/Redis/ES/Milvus/MinIO/NSQ 等依赖跑在 Docker。

### 1) 启动依赖服务

在仓库根目录执行：

```bash
# 先生成 debug env（Makefile 已内置）
make env
# 再起中间件
make middleware
```

如果你使用了 `docker/docker-compose.local.yml` 暴露端口，也可以：

```bash
cd docker
docker compose -f docker-compose.yml -f docker-compose.local.yml up -d
```

### 2) 使用 `.env.local` 管理环境变量（推荐）

`go run main.go` 在宿主机运行时，连接地址需要是 `127.0.0.1`，不要用容器内 DNS（如 `mysql`、`redis`）。

可以把本地开发变量放到 `backend/.env.local`，避免每次手动 `export`。

示例文件：`backend/.env.local`

```bash
LISTEN_ADDR=":8888"
MYSQL_DSN="coze:coze123@tcp(127.0.0.1:3306)/opencoze?charset=utf8mb4&parseTime=True"
REDIS_ADDR="127.0.0.1:6379"
ES_ADDR="http://127.0.0.1:9200"
MINIO_ENDPOINT="127.0.0.1:9000"
MINIO_API_HOST="http://127.0.0.1:9000"
MINIO_AK="minioadmin"
MINIO_SK="minioadmin123"
STORAGE_TYPE="minio"
STORAGE_BUCKET="opencoze"
MILVUS_ADDR="127.0.0.1:19530"
COZE_MQ_TYPE="nsq"
MQ_NAME_SERVER="127.0.0.1:4150"
```

启动前加载：

```bash
cp backend/.env.local backend/.env   # go run 会读取 backend/.env
set -a
source backend/.env.local
set +a
cd backend
go run main.go
```

> 更多变量可参考：`docker/.env.example`

### 3) 启动后端

```bash
cd backend
go run main.go
```

默认监听：`http://localhost:8888`

## 方式二：使用同级 Dockerfile 打包后端镜像

当前目录下提供了用于后端服务的 Dockerfile：

- `backend/Dockerfile`

这个 Dockerfile 的 `COPY` 指令依赖仓库根目录作为构建上下文，例如：

- `COPY backend/...`
- `COPY docker/.env.example ...`

因此不要把 `backend/` 目录本身作为 build context，否则会找不到这些文件。

### 1) 在仓库根目录构建镜像

推荐在仓库根目录执行：

```bash
docker build -f backend/Dockerfile -t coze-backend:latest .
```

如果你当前就在 `backend/` 目录，也可以执行：

```bash
docker build -f Dockerfile -t coze-backend:latest ..
```

### 2) 运行镜像

最简单的运行方式：

```bash
docker run --rm -p 8888:8888 --name coze-backend coze-backend:latest
```

如果你希望显式指定环境文件：

```bash
docker run --rm -p 8888:8888 --name coze-backend \
  --env-file docker/.env.example \
  coze-backend:latest
```

### 3) 与中间件配合使用

这个镜像只打包后端应用本身，不会自动帮你启动 MySQL、Redis、Elasticsearch、Milvus、MinIO、NSQ 等依赖。

如果你本地要联调，建议先在仓库根目录启动依赖服务：

```bash
make middleware
```

然后再运行刚才打好的后端镜像。

### 4) 注意事项

- Dockerfile 当前会复制 `backend/conf`，但没有复制前端静态资源目录 `backend/static`。
- 如果你的使用场景只需要后端 API，这个镜像可以直接使用。
- 如果你希望同时通过这个镜像提供完整站点静态资源，需要确认前端静态资源是否已按你的部署方式一并打入镜像或由其他服务提供。

## 常用开发命令

在仓库根目录执行：

```bash
make server        # 启动后端（开发模式）
make middleware    # 启动依赖中间件
make build_server  # 构建后端
```

## 常见问题

- **连不上 MySQL/Redis**
  - 确认中间件已启动：`make middleware`
  - 若本地运行后端，确认环境变量使用 `127.0.0.1` 而不是 `mysql/redis`
- **模型相关报错**
  - 检查 `backend/conf/model/` 的模型配置是否完整（`api_key`、`model` 等）
