# 设备登录与会话安全

设备登录入口为 `POST /v1/auth/login/device`。已有 `identifier` 登录原账号，新
`identifier` 在注册策略允许时创建账号。`identifier` 必须作为敏感凭据保护，不能把
可公开查询的硬件编号当作可靠身份凭据。本实现没有硬件证明或官方客户端证明。

## 本次升级的兼容性变化

- 启用 `enable_security` 后，请求必须有下述签名；旧的仅 `data` / `time` 格式被拒绝。
- 旧的 `LoginType=device` JWT 没有设备绑定，升级后需重新登录；普通网页会话不受影响。
- 新会话在提供设备标识时绑定具体设备。禁用、解绑、删除或管理员踢下线均撤销该设备
  的全部会话（包括离线会话），不影响同一用户的其他设备。重新启用不恢复旧 Token。
- 已绑定其他账号的标识不能通过邮箱/手机号登录、注册或重置密码自动转移。
  设备游客账号升级应先登录该账号，再使用已有的绑定邮箱/手机号接口，保留原账号数据。
- 设备身份必须通过设备管理接口删除；通用 OAuth/认证方式接口不再操作 `device`。
- `user_agent` 请求体字段不再使用；审计和设备记录读取 HTTP `User-Agent`，IP 读取服务端
  解析出的客户端 IP。设备记录的 UA 保留原有 512 字节上限。

不需要数据库迁移。设备状态以数据库为准，撤销代数与防重放记录保存在现有 Redis 中。

## 加密请求格式

保留原有 AES-CBC 密文格式，增加 Encrypt-then-MAC 完整性保护：

```json
{"data":"BASE64_CIPHERTEXT","time":"HEX_UNIX_NANOSECONDS","sign":"LOWERCASE_HEX_HMAC_SHA256"}
```

`time` 为当前 Unix 纳秒时间戳的小写十六进制表示，不含 `0x` 或前导零。
请求有效期为 5 分钟，最多允许客户端时钟超前 30 秒。同一密钥下的每个请求信封必须
使用不同 `time`；重试也必须重新加密、签名。多个服务实例使用 Redis 原子防重放，
Redis 不可用时拒绝加密请求，不降级放行。

AES 与签名实现已内聚到 `internal/auth/deviceauth`，密文格式保持不变。签名使用独立派生密钥：

```text
mac_key = SHA256(UTF8("ppanel:device-envelope:v1:" + security_secret))
sign = lowercase_hex(HMAC_SHA256(mac_key, UTF8(message)))
```

`message` 为以下六行，以一个换行符 `\n` 分隔，末尾不加换行：

```text
v1
POST
/v1/auth/login/device
body
HEX_UNIX_NANOSECONDS
BASE64_CIPHERTEXT
```

- 第二行为实际 HTTP 方法（大写）；第三行为后端接收到的路径，不包含查询串。
- 第四行为信封位置：请求体用 `body`，查询串用 `query`，响应数据用 `response`。
- 本仓库实现可参考 `internal/auth/deviceauth.Sign`。外部客户端应按上述算法计算签名，
  不应直接依赖服务端的 `internal` 包。
- 查询串只能有 `data`、`time`、`sign` 三项，业务参数放在加密的 JSON 对象中。
  不允许明文参数、重复信封参数，或“加密请求体 + 明文查询参数”混用。
- 同时携带查询串和请求体时，两份信封分别签名并使用不同时间戳。
- 设备登录入口不依赖 `Login-Type` 请求头来启用校验；其他接口可以用
  `Login-Type: device` 请求加密传输。已认证的设备登录会话不能通过省略该头降级为明文。

成功响应仍在顶层 `data` 中返回加密信封，新增 `sign`，签名位置使用 `response`。
客户端必须先验证签名，再解密。加密失败不会返回明文 Token。HTTPS 仍然必需。

## “仅真实设备”字段的含义

`only_real_device` 是历史命名，仅要求上述已认证的设备传输，不代表物理真机、应用
完整性或设备唯一性。共享客户端密钥不能提供这些保证。需要真实设备证明时，应另外
设计每设备密钥与服务端挑战流程，并接入目标平台的证明机制；不要以该布尔值作风控结论。
