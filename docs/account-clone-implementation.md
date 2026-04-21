# 账户复制实现说明

## 1. 目标

新增一个后端接口，支持基于当前登录账户，复制创建一个新的用户。

输入参数：

- 新用户邮箱
- 新用户密码

核心要求：

- 以当前登录用户为源用户
- 创建一个新的目标用户
- 一比一复制源账户下的空间与资源
- 保持资源归属关系、引用关系、空间归属关系
- 复制失败时尽可能回滚已创建的新用户和新资源

## 2. API 入口

### 路由

- `POST /api/passport/account/clone/`

### 入口文件

- `backend/api/handler/coze/account_clone.go`
- `backend/api/router/account_clone.go`
- `backend/api/router/register.go`

### 请求体

```json
{
  "email": "new-user@example.com",
  "password": "your-password"
}
```

### 返回体

```json
{
  "code": 0,
  "data": {
    "user_id": "1234567890",
    "summary": {
      "spaces": 1,
      "projects": 2,
      "bots": 3,
      "workflows": 4,
      "plugins": 5,
      "knowledge": 6,
      "databases": 7
    }
  }
}
```

## 3. 总体实现结构

主编排入口在：

- `backend/application/accountclone/service.go`

它负责把整个流程串起来：

1. 校验当前登录态和入参
2. 读取源用户信息与所属空间
3. 搜索源用户全部需要复制的资源
4. 生成目标用户 ID
5. 先创建目标空间
6. 创建目标用户并绑定到克隆后的空间
7. 按资源类型执行深拷贝
8. 任一环节失败时执行补偿回滚

## 4. 主流程说明

### 4.1 请求校验

在 `CloneCurrentAccount` 中先做基础检查：

- 必须存在登录 session
- 新邮箱格式合法
- 新密码不能为空

若没有 session，会返回权限错误。

### 4.2 获取源用户与源空间

通过：

- `backend/application/user/clone_support.go`

中的 `GetCloneSource` 获取：

- 源用户基础信息：名称、描述、语言
- 源用户所属空间列表

这一步是后续资源复制的基础。

### 4.3 收集待复制资源

`collectAssets` 会遍历源用户所有空间，通过搜索域服务收集：

- 项目
- Bot
- 工作流
- 插件
- 知识库
- 数据库

同时收集这些资源实际依赖到的空间 ID，避免复制无关空间。

资源搜索依赖：

- `searchProjectsByType`
- `searchLibraryResources`

### 4.4 创建目标用户与目标空间

执行顺序是：

1. 先生成目标用户 ID
2. 先创建克隆后的空间
3. 再创建目标用户
4. 再把用户补充绑定到其它克隆空间

这样做的原因是：

- 用户创建本身需要一个主空间 ID
- 后续复制资源时又需要目标空间已存在

对应能力分别落在：

- `GenerateCloneTargetUserID`
- `CloneSpacesForUser`
- `BindUserToClonedSpaces`
- `DomainSVC.Create`

## 5. 资源复制逻辑

主资源复制入口：

- `copyAssets`

复制顺序如下：

1. 项目
2. 插件
3. 知识库
4. 数据库
5. 工作流
6. Bot

之所以 Bot 放在最后，是因为 Bot 依赖插件、知识库、数据库、工作流，必须先把这些底层资源复制出来，Bot 才能正确重写引用。

### 5.1 项目复制

通过：

- `backend/application/app/clone.go`

新增的 `CloneDraftProjectToUser` 完成。

关键点：

- 复制 draft project 到目标空间
- 发布搜索事件，保证新项目在检索侧可见
- 新项目 owner/space 归属改为目标用户和目标空间

### 5.2 插件复制

插件复制复用了原有插件复制能力，但补齐了目标空间归属：

- `backend/domain/plugin/dto/plugin.go`
- `backend/domain/plugin/service/plugin_online.go`
- `backend/application/app/app.go`

关键改动：

- `CopyPluginRequest` 增加 `TargetSpaceID`
- 复制后的插件 `SpaceID` 能正确落到目标空间
- 避免复制出来的插件仍挂在原空间或 APP 上

### 5.3 数据库复制

数据库复制同样补齐了目标空间归属：

- `backend/application/app/app.go`

关键改动：

- `CopyDatabaseRequest` 增加 `TargetSpaceID`
- 复制数据库时不仅复制数据，也把资源归属到目标空间

### 5.4 工作流复制

工作流复制是最复杂的一块，核心在：

- `backend/application/workflow/workflow.go`

新增主入口：

- `CloneLibraryWorkflowToSpace`

它做了这些事情：

1. 用目标用户身份构造上下文，避免 owner 校验沿用源用户
2. 递归分析工作流依赖的子工作流
3. 递归复制依赖的插件、知识库、数据库
4. 重写工作流 canvas 中的资源引用
5. 复制工作流草稿
6. 同步源工作流的发布态到目标工作流

关键修复点：

- 修复了工作流 owner 不匹配导致的权限错误
- 修复了子工作流递归复制中的引用重写
- 修复了 Bot 关联 workflow 时需要读取最新发布版本的问题

### 5.5 知识库复制

知识库复制复用了现有 deep copy 能力，核心链路在：

- `backend/domain/knowledge/service/datacopy.go`

复制内容包括：

- 知识库元数据
- 文档
- 文档切片
- 表格型知识库数据
- SearchStore 索引数据

本次为了保证复制稳定性，补了两个关键修复：

1. 文档筛选从“只复制 Enable/Init”改为“复制所有非 Deleted 文档”
2. 先写入 document/slice，再写 SearchStore

这样解决了两个问题：

- 某些可见文档之前会被漏复制
- 当 embedding/SearchStore 环境不完整时，不再让整个知识库复制直接失败

当前行为是：

- 文档与切片优先落库
- SearchStore 写入失败时降级为文档索引失败，不阻断整个知识库复制

### 5.6 Bot 复制

Bot 深拷贝相关逻辑最终收敛到了：

- `backend/application/singleagent/clone_helpers.go`

主流程为：

1. 基于源 Bot draft 生成新的 Bot draft
2. 复制变量元信息
3. 复制 Bot 依赖的插件与工具
4. 复制 Bot 依赖的知识库
5. 复制 Bot 依赖的数据库
6. 复制 Bot 依赖的工作流
7. 重写 Bot 中所有资源 ID 引用
8. 复制快捷指令
9. 保存新的 draft bot 并发布搜索事件

这样避免了把账户复制的特殊需求耦合进通用 duplicate 逻辑，防止引入 import cycle 和通用路径污染。

## 6. 去重策略

在资源深拷贝过程中，出现过“同一个资源被重复复制”的问题，最终通过共享映射表解决。

### 6.1 工作流去重

使用：

- `workflowIDMap`

解决来源：

- 库资源阶段复制一次 workflow
- Bot 依赖阶段又复制一次 workflow

现在会优先查表，已复制过则直接复用目标 workflow ID。

### 6.2 知识库去重

使用：

- `knowledgeIDMap`

解决来源：

- 库资源阶段复制一次 knowledge
- Bot 依赖阶段又复制一次 knowledge

### 6.3 仍需关注的点

从当前实现看，以下场景仍需要后续继续观察或进一步治理：

- 插件在“库资源复制”和“Bot 依赖复制”之间仍可能存在重复复制风险
- 数据库在“库资源复制”和“Bot 依赖复制”之间仍可能存在重复复制风险
- 工作流内部外部资源的去重目前是单次 workflow clone 递归内共享，非全局账户克隆级别统一去重

## 7. 回滚策略

由于这条链路跨多个领域服务和存储，不适合用单个数据库事务直接包住，因此最终采用“补偿回滚”。

入口：

- `rollbackClonedAccount`

策略：

- 若目标用户创建后任一阶段失败，则开始清理
- 按资源类型反向删除目标空间下的 Bot、项目、工作流、插件、知识库、数据库
- 最后删除目标用户、space_user、space 等用户域数据

相关文件：

- `backend/application/accountclone/service.go`
- `backend/application/user/clone_support.go`
- `backend/domain/user/repository/repository.go`
- `backend/domain/user/internal/dal/user.go`
- `backend/domain/user/service/user.go`
- `backend/domain/user/service/user_impl.go`

这是“尽力回滚”而不是严格分布式事务，但已经能覆盖本次功能涉及的大多数已创建资源。

## 8. 关键演进过程

本功能并不是一次性完成的，而是经过多轮修复后稳定下来的。

### 阶段一：打通基本链路

完成内容：

- 新增账户复制接口
- 先能创建新用户
- 先能复制空间、项目、Bot 等主资源

### 阶段二：补齐深拷贝

完成内容：

- 补齐 Bot 依赖资源的复制
- 补齐工作流、知识库、数据库、插件复制
- 处理资源引用重写

### 阶段三：修复运行时问题

修复内容：

- 克隆空间后用户未绑定导致访问异常
- 工作流 owner 不匹配导致权限错误
- plugin copy 某些校验路径异常
- workflow clone nil 指针问题

### 阶段四：修复重复复制

修复内容：

- workflow 被复制两次
- knowledge 被复制两次

对应方案：

- 加入全局共享 ID map 去重

### 阶段五：修复发布态与资源完整性

修复内容：

- Bot 依赖的 workflow 需要同步 published latest version
- knowledge 文档之前复制不全

### 阶段六：补偿回滚

修复内容：

- 整条链路任一点失败时，删除已创建目标用户及其资源

### 阶段七：修复知识库 SearchStore 降级问题

修复内容：

- embedding 环境缺失时，知识库复制不再整单失败
- 先落 document/slice，再尝试建索引

## 9. 当前最终落地文件

### 新增文件

- `backend/api/handler/coze/account_clone.go`
- `backend/api/router/account_clone.go`
- `backend/application/accountclone/service.go`
- `backend/application/app/clone.go`
- `backend/application/singleagent/clone_helpers.go`
- `backend/application/user/clone_support.go`

### 修改文件

- `backend/api/router/register.go`
- `backend/application/app/app.go`
- `backend/application/user/init.go`
- `backend/application/user/user.go`
- `backend/application/workflow/workflow.go`
- `backend/domain/knowledge/service/datacopy.go`
- `backend/domain/plugin/dto/plugin.go`
- `backend/domain/plugin/service/plugin_online.go`
- `backend/domain/user/repository/repository.go`
- `backend/domain/user/internal/dal/user.go`
- `backend/domain/user/service/user.go`
- `backend/domain/user/service/user_impl.go`

## 10. 当前实现的边界与已知注意点

1. 回滚是补偿式回滚，不是强事务。
2. 插件和数据库的跨阶段去重还可以继续加强。
3. 知识库复制在 SearchStore/embedding 不可用时会降级，不保证复制后立即可检索，但可以保证文档与切片先保留下来。
4. 工作流复制依赖目标用户上下文，不能沿用源用户 session。

## 11. 一句话总结

这次“复制用户”功能最终落地成了一条跨用户域、空间域、项目域、Bot 域、工作流域、插件域、知识库域、数据库域的编排链路：

- 先建目标用户与空间
- 再按依赖顺序深拷贝资源
- 复制过程中统一重写引用关系
- 出错时执行补偿回滚
- 对知识库索引等外部依赖场景做降级容错

最终目标是让新账户能够拿到一个尽可能完整、归属正确、引用闭合的资源副本。
