# 进程守护使用手册

> 适用于 Windows 平台，用于自动守护业务系统进程，支持自动重启、升级暂停、双服务互保、Web 控制台。

---

## 一、软件组成

| 文件 | 作用 |
|---|---|
| `daemon-go.exe` | 守护核心程序，A 和 B 两个服务都是同一个文件 |
| `config.yaml` | 守护配置文件 |
| `icon.ico` | 图标 |
| `scripts/install-service.bat` | 安装 A/B 服务 |
| `scripts/uninstall-service.bat` | 卸载 A/B 服务 |
| `scripts/restart-service.bat` | 重启 A/B 服务 |
| `scripts/start-manager.bat` | 打开 Web 控制台（自动携带 Token） |
| `.guard` | 自动生成的访问令牌文件，请勿泄露 |

---

## 二、部署步骤

### 1. 准备目录

将以下文件放到业务系统同级目录，例如：

```text
D:\YourApp\
├── daemon-go.exe
├── config.yaml
├── icon.ico
├── scripts\
│   ├── install-service.bat
│   ├── uninstall-service.bat
│   ├── restart-service.bat
│   └── start-manager.bat
├── Start.bat
├── Stop.bat
└── ...
```

### 2. 修改配置文件

编辑 `config.yaml`，主要修改 `projects` 部分。

### 3. 安装服务

**右键点击 `scripts/install-service.bat`，选择"以管理员身份运行"。**

安装完成后会自动启动两个服务：

- `ProcessGuard-A`：主守护，守护业务项目，提供 Web 控制台
- `ProcessGuard-B`：备守护，监控 A 服务

两个服务都设置为**开机自动启动**。

### 4. 打开 Web 控制台

服务启动后，**双击运行 `scripts/start-manager.bat`**，会自动使用默认浏览器打开控制台并携带访问令牌。

首次安装后，也可以在安装目录下找到自动生成的 `.guard` 文件，其中 `token=` 后面的字符串即为访问令牌。直接访问：

```text
http://127.0.0.1:18080/?token=你的Token
```

> Web 控制台**强制要求 Token**，并且**只允许 127.0.0.1 / ::1 本机访问**，远程机器无法操作。

---

## 三、配置文件详解

`config.yaml` 分为三部分：实例配置、对端互保配置、项目配置。

```yaml
# 当前实例，a 为主守护，b 为备守护
# 命令行参数 --instance 会覆盖此值
instance: a

# 日志目录，自动创建
logDir: logs

# 暂停/运行状态文件，自动创建
stateFile: daemon.state

# Web 控制台监听地址（仅本机）
webHost: 127.0.0.1
webPort: 18080

# 对端互保配置
peer:
  instanceId: b
  checkInterval: 10

# runCommand 动作允许列表（为空时禁止 runCommand）
allowedCommands: []

# 被守护项目列表
projects:
  - name: YourApp
    enabled: true
    monitor:
      type: port
      port: 8080
      host: 127.0.0.1
    checkInterval: 15
    startupTimeout: 60
    maxRestartAttempts: 3
    restartDelay: 10
    startScript: "Start.bat"
    stopScript: "Stop.bat"
    preActions:
      - type: killProcess
        params:
          processName: old-process.exe
      - type: wait
        params:
          millis: "1000"
    postActions:
      - type: killWindow
        params:
          windowTitle: old-window
      - type: wait
        params:
          millis: "2000"
```

### 字段说明

| 字段 | 说明 | 示例 |
|---|---|---|
| `name` | 项目名称 | `YourApp` |
| `enabled` | 是否启用守护 | `true` / `false` |
| `monitor.type` | 监控方式：`port` 端口、`process` 进程名、`mixed` 任一满足 | `port` |
| `monitor.port` | 监控端口 | `8080` |
| `monitor.processName` | 监控进程名，`type` 为 process 或 mixed 时有效 | `app.exe` |
| `checkInterval` | 检查间隔（秒） | `15` |
| `startupTimeout` | 启动超时（秒） | `60` |
| `maxRestartAttempts` | 连续失败最大次数 | `3` |
| `restartDelay` | 失败后的重试间隔（秒） | `10` |
| `startScript` | 项目启动脚本路径，支持相对路径和绝对路径 | `Start.bat` / `D:\App\Start.bat` |
| `stopScript` | 项目停止脚本路径（可选） | `Stop.bat` / `D:\App\Stop.bat` |
| `preActions` | 启动前执行的动作列表 | 见下方动作说明 |
| `postActions` | 启动成功后执行的动作列表 | 见下方动作说明 |
| `allowedCommands` | `runCommand` 允许列表，未配置时禁止执行任意命令 | 见下方安全说明 |

### 前后置动作说明

每个动作由 `type` 和 `params` 组成。

#### `killProcess`：按进程名结束进程

```yaml
- type: killProcess
  params:
    processName: old-process.exe
```

用于启动前清理残留进程。

#### `killPort`：按端口结束占用进程

```yaml
- type: killPort
  params:
    port: "8080"
```

#### `killWindow`：按窗口标题关闭窗口

```yaml
- type: killWindow
  params:
    windowTitle: old-window
```

用于启动后关闭弹出的黑框窗口。

#### `wait`：等待指定毫秒

```yaml
- type: wait
  params:
    millis: "1000"
```

#### `runCommand`：执行允许的命令（受 allowlist 约束）

```yaml
allowedCommands:
  - path: "D:\\Tools\\kill-xjar.bat"
    argsPattern: ""

preActions:
  - type: runCommand
    params:
      command: "D:\\Tools\\kill-xjar.bat"
      workDir: ""
```

> **安全说明**：`runCommand` 必须先在 `allowedCommands` 中声明可执行文件的**绝对路径**，并可选配置参数正则 `argsPattern`。未配置允许列表时，`runCommand` 会拒绝执行任何命令，防止任意命令执行。

---

## 四、运行方式

### 方式一：作为 Windows 服务运行（推荐）

执行 `scripts/install-service.bat` 后，系统会注册两个服务：

```text
ProcessGuard-A
ProcessGuard-B
```

这两个服务会：

- 开机自动启动
- 不依赖用户登录
- 无窗口、无托盘，对客服不可见

### 方式二：控制台临时运行（调试用）

```bash
# 运行 A 实例
daemon-go.exe --instance a

# 运行 B 实例
daemon-go.exe --instance b
```

注意：同一实例只能运行一个，第二个启动会提示"已在运行"并退出。

---

## 五、日常操作

### 通过 Web 控制台

双击 `scripts/start-manager.bat` 打开控制台（自动携带 Token），或手动访问：

```text
http://127.0.0.1:18080/?token=你的Token
```

控制台页面可执行：

- 查看守护状态
- 暂停 / 恢复守护
- 查看每个项目状态（PID、启动时间、连续失败次数）
- 手动启动 / 停止 / 重启单个项目
- 重置项目失败计数
- **可视化编辑并保存 `config.yaml` 配置**
- 查看最近关键事件

> Web 控制台默认仅监听 `127.0.0.1`，且必须提供 Token，不允许远程访问。

### 在 Web 控制台修改配置

1. 在"配置管理"区域填写全局设置和项目信息
2. 点击每个项目可展开/折叠详细配置
3. 可添加、删除项目
4. 每个项目可配置前置动作和后置动作
5. 点击"保存配置"
6. 保存成功后，点击"立即重启"，由对端 B 自动拉起 A 服务生效

> 原配置会自动备份为 `config.yaml.bak`。

### 常用命令

```bash
# 查看服务状态
sc query ProcessGuard-A
sc query ProcessGuard-B

# 手动启动/停止服务
sc start ProcessGuard-A
sc stop ProcessGuard-A

# 快速重启两个服务（以管理员身份运行）
scripts\restart-service.bat
```

---

## 六、升级维护流程

升级业务系统时，建议按以下步骤操作：

1. 打开 Web 控制台，点击**暂停守护**
2. 确认状态变为"已暂停"
3. 手动停止业务系统，替换程序文件
4. 手动启动业务系统验证
5. 返回 Web 控制台，点击**恢复守护**

> 暂停状态会保存到 `daemon.state` 文件。即使升级期间主守护 A 意外崩溃，被备守护 B 拉起后仍会保持暂停状态，不会自动重启业务系统。

---

## 七、双守护互保说明

- **A 服务**：主守护，负责守护所有业务项目，提供 Web 控制台
- **B 服务**：备守护，只负责监控 A 服务是否存活
- A 崩溃或停止时，B 会通过 `sc start ProcessGuard-A` 重新拉起 A
- A 被拉起后读取 `daemon.state`，保持原来的暂停或运行状态
- 两个服务都通过 `daemon-go.exe` 运行，只是启动参数不同

---

## 八、连续失败保护

- 项目启动失败时，会连续尝试 `maxRestartAttempts` 次
- 达到上限后，状态变为 `FAILED`，停止自动重启
- 此时需要人工排查原因
- 排查完成后，可在 Web 控制台点击"重置失败"或"重启"恢复守护

---

## 九、日志与事件

### 日志文件

日志文件位于 `logs/` 目录：

- `daemon-a-YYYYMMDD.log`：主守护详细日志
- `daemon-b-YYYYMMDD.log`：备守护详细日志

详细日志内容包括：

- 服务启动/停止
- 项目启动、停止、状态变化
- 前后置动作执行
- 暂停/恢复命令
- 异常错误

详细日志按天自动分片，单个文件最大 10MB，保留最近 30 天。

### 关键事件

关键事件保存在：

- `logs/events-a.jsonl`
- `logs/events-b.jsonl`

只记录关键节点，例如：

- 本端 a 检测到对端 b 掉线 / 恢复
- 项目掉线 / 启动 / 启动成功 / 连续失败 / 停止
- 手动启动 / 停止 / 重启
- 暂停 / 恢复守护
- 配置保存 / 服务重启

每个实例最多保留 100 条事件，Web 控制台展示合并后的最近 10 条，便于客服快速定位问题。

---

## 十、常见问题

### Q1：安装服务时提示权限不足

A：必须以**管理员身份**运行 `install-service.bat`。右键点击 bat 文件，选择"以管理员身份运行"。

### Q2：Web 控制台打不开

A：按以下步骤排查：

1. 检查 A 服务是否运行：`sc query ProcessGuard-A`
2. 检查端口 18080 是否被占用：`netstat -ano | findstr :18080`
3. 检查 `config.yaml` 中的 `webPort` 是否配置正确
4. 检查 `.guard` 文件是否存在

### Q3：为什么暂停后 A 崩溃被拉起，状态还是暂停

A：这是设计行为。暂停状态会持久化，防止升级期间自动重启业务系统。

### Q4：黑框窗口无法关闭

A：在 `postActions` 中配置 `killWindow`，并确认 `windowTitle` 与实际窗口标题一致。也可以改用 `javaw` 启动减少黑框。

### Q5：如何卸载服务

A：右键以管理员身份运行 `scripts/uninstall-service.bat`。

### Q6：Web 控制台修改配置后为什么不立即生效

A：当前运行中的守护进程不会自动热加载配置，避免运行时配置变更导致异常。保存后点击"立即重启"，由 B 自动拉起 A 服务生效。

### Q7：启动脚本支持绝对路径吗

A：支持。如果 `startScript` / `stopScript` 是绝对路径，守护进程会自动将工作目录设置为脚本所在目录；如果是相对路径，则相对于守护进程所在目录。

### Q8：忘记 Token 怎么办

A：Token 保存在安装目录的 `.guard` 文件中，格式为 `token=xxxxxxxx`。可直接读取或删除该文件后重启 A 服务，系统会重新生成新的 Token。

### Q9：想换一台机器管理 Web 控制台

A：不支持。Web 控制台强制绑定 127.0.0.1 / ::1，只允许本机浏览器访问，防止外部机器操作。

### Q10：为什么配置了 runCommand 却不执行

A：`runCommand` 必须先在 `allowedCommands` 中声明可执行文件的绝对路径。未配置时，任何 `runCommand` 都会被拒绝。

---

## 十一、完整配置示例

```yaml
instance: a
logDir: logs
stateFile: daemon.state
webHost: 127.0.0.1
webPort: 18080

peer:
  instanceId: b
  checkInterval: 10

# runCommand 动作允许列表（为空时禁止 runCommand）
allowedCommands: []

projects:
  - name: YourApp
    enabled: true
    monitor:
      type: port
      port: 8080
      host: 127.0.0.1
    checkInterval: 15
    startupTimeout: 60
    maxRestartAttempts: 3
    restartDelay: 10
    startScript: "Start.bat"
    stopScript: "Stop.bat"
    preActions:
      - type: killProcess
        params:
          processName: old-process.exe
      - type: wait
        params:
          millis: "1000"
    postActions:
      - type: killWindow
        params:
          windowTitle: old-window
      - type: wait
        params:
          millis: "2000"
```

---

## 十二、维护命令

- 重启 A/B 服务：
  ```bash
  scripts\restart-service.bat
  ```
- 查看实时日志：
  ```bash
  tail -f logs\daemon-a-YYYYMMDD.log
  ```
- 查看关键事件：
  ```bash
  type logs\events-a.jsonl
  ```
