# Coze Studio 后端架构梳理

## 1. 文档目的

这份文档用于从实现视角梳理 Coze Studio 的后端架构，重点回答四个问题：

1. 后端整体如何分层。
2. 数据分别存在哪里。
3. 请求进入后如何读取数据。
4. 搜索、筛选、提取类能力是如何落地的。

本文不是泛化的目录说明，而是结合当前代码实现来说明关键链路。

---

## 2. 整体架构概览

后端采用的是一种偏 DDD 的分层架构，HTTP 接入、业务编排、领域规则、跨域门面、基础设施实现彼此分离。

核心目录职责如下：

- `backend/api/`
  - HTTP 接口层。
  - 包含路由、handler、中间件、请求参数绑定、响应封装。
- `backend/application/`
  - 用例编排层。
  - 负责把多个领域服务串成一个完整业务流程。
- `backend/domain/`
  - 领域层。
  - 包含实体、领域服务、仓储接口、核心业务规则。
- `backend/crossdomain/`
  - 跨域调用门面。
  - 用于隔离模块间直接依赖，降低耦合。
- `backend/infra/`
  - 基础设施层。
  - 提供数据库、缓存、对象存储、搜索引擎、向量库、消息队列、文档解析器等实现。

从运行链路上看，可以简化为：

HTTP Request -> API 层 -> Application 层 -> Domain 层 -> Infra 层 -> MySQL / Redis / ES / Milvus / OSS / MQ

---

## 3. 启动与装配

### 3.1 启动入口

启动入口在 `backend/main.go`。

主流程是：

1. 加载环境变量。
2. 初始化日志级别。
3. 调用 `application.Init(ctx)` 完成依赖装配。
4. 启动 Hertz HTTP 服务。

其中中间件顺序是固定的，先做上下文缓存、请求检查、Host 注入、LogID、CORS、访问日志、OpenAPI 鉴权、Session 鉴权，再做 I18n。

### 3.2 服务装配中心

真正的依赖装配入口在 `backend/application/application.go`。

这里分三层初始化：

- `initBasicServices`
  - 初始化最基础的服务，主要依赖 infra。
- `initPrimaryServices`
  - 初始化知识库、工作流、插件、内存等核心领域服务。
- `initComplexServices`
  - 初始化 Bot、搜索、会话、项目等更高层编排服务。

装配完成后，会把一部分能力注册到 `crossdomain`，供其他模块通过统一门面调用。

这意味着项目不是“谁需要就直接 new 谁”，而是先在启动阶段集中装配，再通过依赖注入的方式分发到各模块。

---

## 4. 基础设施与数据存储介质

基础设施初始化逻辑位于 `backend/application/base/appinfra/app_infra.go`。

这个文件几乎可以视为整个系统的数据与能力底座说明书。

### 4.1 MySQL：业务真相层

MySQL 初始化在 `backend/infra/orm/impl/mysql/mysql.go`。

它使用 GORM 建立连接，并配置连接池。

MySQL 存储的是系统中的主业务数据，例如：

- 用户、空间、权限关系
- 项目、Bot、工作流元数据
- 插件配置
- 知识库元数据
- 文档记录
- 文档切片记录
- 表格型知识库结构与数据关系

结论：

MySQL 是业务数据的事实来源。搜索索引、向量索引都不是主存储，而是派生索引层。

### 4.2 Redis：缓存与状态辅助层

Redis 初始化在 `backend/infra/cache/impl/redis/redis.go`。

Redis 在当前系统中主要承担：

- 缓存
- 计数器
- 分布式 ID 生成的辅助能力
- 一些轻量状态的快速访问

它不是主业务存储，而是提升性能和支撑并发的辅助层。

### 4.3 对象存储：文件与二进制对象层

对象存储初始化在 `backend/infra/storage/impl/storage.go`。

当前支持：

- MinIO
- TOS
- S3

对象存储主要保存：

- 上传文件
- 文档原文件
- 图片、头像、图标
- 静态资源引用对象

也就是说，文件不会直接塞进 MySQL，而是存储对象 URI，业务表里保留元信息与引用关系。

### 4.4 Elasticsearch：全文搜索与资源筛选层

ES 客户端初始化也在 `backend/application/base/appinfra/app_infra.go`，搜索实现落在：

- `backend/domain/search/service/search.go`

ES 主要负责：

- 项目搜索
- 资源库搜索
- 名称匹配
- 类型筛选
- 发布状态筛选
- 排序和分页
- 知识库全文检索的一部分能力

### 4.5 Milvus：向量检索层

Milvus 相关实现位于：

- `backend/infra/document/searchstore/impl/milvus/milvus_searchstore.go`

它主要承担知识库语义检索，即把 query 和文档切片都转成向量后进行相似度召回。

### 4.6 消息队列：异步索引与事件驱动层

事件总线由 `backend/application/application.go` 中的 `initEventBus` 初始化。

搜索模块会注册资源和项目 consumer，知识库模块也会注册自己的 consumer。

这意味着很多“写业务数据”和“更新索引”的动作不是强同步绑定，而是通过消息异步传播完成。

---

## 5. 分层职责如何协作

### 5.1 API 层做什么

API 层只处理入口问题，例如：

- 请求参数绑定
- session 获取
- 路由分发
- 返回码和响应格式

它尽量不承载重业务逻辑。

### 5.2 Application 层做什么

Application 层负责“一个用户动作到底要串几个领域模块”。

例如：

- 复制账户
- 创建 Bot 并关联工作流、知识库、插件
- 搜索资源后再补齐头像、图标、行为权限

它的重点不是实现底层算法，而是业务流程编排。

### 5.3 Domain 层做什么

Domain 层负责：

- 领域模型
- 规则校验
- 仓储读写
- 领域内部转换

例如知识库模块会定义文档、切片、检索策略、过滤条件等领域对象，并由领域服务统一处理。

### 5.4 Infra 层做什么

Infra 层负责外部系统对接与技术实现，例如：

- MySQL DAO
- Redis client
- ES client
- Milvus search store
- parser / OCR / rerank / NL2SQL

领域层通常只知道自己需要一个能力接口，不关心底层用的是哪家组件。

---

## 6. 数据如何存储

### 6.1 结构化业务数据

结构化业务数据大多存 MySQL。

典型模式是：

1. API 或 Application 发起请求。
2. Domain Service 调用 Repository / DAO。
3. DAO 基于 GORM 读写 MySQL。

例如知识库切片 DAO 在：

- `backend/domain/knowledge/internal/dal/dao/knowledge_document_slice.go`

可以看到切片的创建、批量创建、按文档查询、按条件筛选等操作都先落在 MySQL。

这说明切片本身并不是只存在于向量库里，而是先作为业务记录持久化。

### 6.2 非结构化原始文件

原始文件存对象存储，数据库只存元信息和 URI。

这类数据一般包括：

- PDF / Word / Markdown / CSV / XLSX 原文档
- 图片资源
- 用户上传附件

这样可以避免数据库承载大量二进制数据。

### 6.3 搜索索引数据

搜索索引不是事实源，而是从业务数据派生出来的可检索副本。

其中：

- 资源、项目检索索引主要在 ES
- 知识切片语义索引主要在 Milvus

换句话说，索引是为了提高查找和召回效率，而不是主数据存储。

---

## 7. 数据如何读取

### 7.1 普通业务读取

普通业务读取通常是：

1. API 层收请求。
2. Application 层判断业务上下文。
3. Domain 层调用仓储读取 MySQL。
4. 必要时拼装对象存储 URL、权限、状态信息。
5. 返回前端。

### 7.2 搜索型读取

搜索型读取一般不会直接扫 MySQL，而是走索引层。

典型入口在：

- `backend/application/search/resource_search.go`

它先构造搜索请求，再交给领域搜索服务。

真正执行筛选逻辑的是：

- `backend/domain/search/service/search.go`

这里会把请求转成 ES 查询条件。

### 7.3 知识库读取

知识库读取往往分两步：

1. 先用 MySQL 过滤“哪些知识库和文档是允许参与检索的”。
2. 再用 ES / Milvus / NL2SQL 做召回。

实现入口在：

- `backend/domain/knowledge/service/retrieve.go`

这里的 `prepareRAGDocuments` 会先过滤：

- 启用中的 knowledge
- 状态为 enable 的 document

之后才进入检索阶段。

这一步非常关键，说明索引命中不代表一定可返回，真正是否允许参与召回，仍然受 MySQL 中业务状态控制。

---

## 8. 数据如何筛选

### 8.1 资源搜索的筛选方式

在 `backend/domain/search/service/search.go` 中，资源搜索会把筛选条件下推给 ES。

常见筛选维度包括：

- `space_id`
- `app_id`
- `owner_id`
- `name`
- `res_type`
- `res_sub_type`
- `publish_status`

同时支持：

- 排序字段
- 升序 / 降序
- 分页
- 游标翻页

因此资源列表、项目列表这类“库检索”能力，本质上是索引筛选，而不是查库后在内存里手工过滤。

### 8.2 知识库的筛选方式

知识库召回的筛选更加复杂，包含两层：

第一层是业务过滤：

- 知识库是否启用
- 文档是否启用
- 当前请求中传入的 knowledgeIDs / documentIDs

第二层是检索过滤：

- 向量召回
- 全文召回
- 表格知识的 NL2SQL 召回

最后再统一做 rerank。

所以知识库不是单一搜索引擎直出，而是“业务条件过滤 + 多通道召回 + 重排”的组合流程。

---

## 9. 数据如何提取

“提取”在这个项目里主要对应文档解析、切片、结构抽取和检索前预处理。

相关能力在：

- `backend/infra/document/parser/manager.go`

可以看到系统支持多种文件格式，并支持多种提取策略：

- PDF / TXT / DOC / DOCX / Markdown
- CSV / XLSX / JSON
- JPG / JPEG / PNG
- 图片 OCR
- 表格抽取
- 页码过滤
- 分块策略
- 自定义 chunk size / separator / overlap
- 分层切片

这意味着原始文档进入系统后，不会直接整篇塞进索引，而是先经过解析与切片，形成更适合检索与召回的结构化切片。

---

## 10. 知识库的完整数据链路

知识库是这个后端里最典型、也最能体现“存储、读取、筛选、提取”的模块。

### 10.1 写入链路

典型流程是：

1. 原始文件上传到对象存储。
2. 文档元信息写入 MySQL。
3. 文档经过 parser 解析。
4. 文本、表格、图片等内容被抽取。
5. 内容被切成多个 slice。
6. slice 记录先写入 MySQL。
7. slice 再被转成检索文档。
8. 检索文档写入文本索引和向量索引。

在 `backend/domain/knowledge/service/datacopy.go` 中可以直接看到类似模式：

- 先 `BatchCreate` 切片记录
- 再 `slice2Document`
- 最后调用 search store 的 `Store`

这说明数据库优先于索引，索引只是后置步骤。

### 10.2 读取链路

知识库检索入口在 `backend/domain/knowledge/service/retrieve.go`。

整体流程是：

1. 校验请求。
2. 构造检索上下文。
3. 先从 MySQL 过滤出可用 knowledge 和 document。
4. 可选做 query rewrite。
5. 并行执行：
   - 向量召回
   - ES 召回
   - NL2SQL 召回
6. 做 rerank。
7. 打包返回切片结果。

这是一条典型的 RAG 检索链路。

### 10.3 向量召回如何落地

Milvus 检索实现在：

- `backend/infra/document/searchstore/impl/milvus/milvus_searchstore.go`

这里会：

1. 把 query 做 embedding。
2. 构造向量检索请求。
3. 按 collection / partition 检索。
4. 返回命中的文档切片。

对于混合检索场景，还支持 dense + sparse 的组合方式。

---

## 11. 资源搜索的完整数据链路

资源搜索是另一个比较清晰的例子。

入口在：

- `backend/application/search/resource_search.go`

流程如下：

1. 从上下文中取当前用户。
2. 组装 `SearchResourcesRequest`。
3. 调用领域搜索服务。
4. 领域搜索服务把条件转成 ES bool query。
5. 取回索引文档。
6. 应用层补齐 icon、creator、actions、业务状态等附加信息。
7. 返回前端资源列表。

这里可以看出一个典型分工：

- ES 负责快速找出候选资源。
- Application 层负责补全展示数据。
- User / TOS / 其他领域服务负责提供附加信息。

---

## 12. 这个后端的设计特点

从当前实现看，这个后端有几个很明显的特点：

### 12.1 多存储混合架构

不是单库单表模型，而是：

- MySQL 存主业务数据
- Redis 存缓存和辅助状态
- OSS 存文件
- ES 存全文和结构化检索索引
- Milvus 存语义向量索引

### 12.2 业务真相与索引解耦

MySQL 是业务真相层，ES 和 Milvus 是检索层。

检索层命中只是候选结果，最终是否可返回，仍然受业务状态控制。

### 12.3 编排与规则分离

Application 层专注编排，Domain 层专注规则，Infra 层专注实现。

这种结构对复杂业务比较友好，尤其适合知识库、工作流、Bot 这类跨模块协同很强的场景。

### 12.4 检索链路是混合式的

尤其在知识库模块中，不是单独依赖向量检索，而是会结合：

- Query Rewrite
- ES 全文检索
- Vector Search
- NL2SQL
- Rerank

这说明系统设计目标不是只做“存数据”，而是面向智能问答和 RAG 检索质量做了专门建设。

---

## 13. 一句话总结

Coze Studio 的后端可以概括为：

以 MySQL 为业务真相层，以 Redis 为缓存辅助层，以对象存储保存原始文件，以 ES 和 Milvus 承担检索能力，再通过 Application + Domain 分层把用户、项目、Bot、工作流、知识库等复杂业务编排起来。

其中最关键的设计点是：

- 业务数据和检索索引分离
- 文档先解析和切片，再进入检索体系
- 检索前先做业务状态过滤
- 检索后再做重排和结果组装

---

## 14. 建议阅读顺序

如果要继续深入理解实现，建议按以下顺序读代码：

1. `backend/main.go`
2. `backend/application/application.go`
3. `backend/application/base/appinfra/app_infra.go`
4. `backend/domain/search/service/search.go`
5. `backend/application/search/resource_search.go`
6. `backend/domain/knowledge/service/retrieve.go`
7. `backend/domain/knowledge/internal/dal/dao/knowledge_document_slice.go`
8. `backend/infra/document/searchstore/impl/milvus/milvus_searchstore.go`
9. `backend/infra/document/parser/manager.go`

读完这几处，基本就能建立对后端主干、数据流向和检索链路的整体理解。
