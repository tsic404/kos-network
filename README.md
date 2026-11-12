# kos-network

KOS 系统应用项目：网络视图扩展包 — 入口、证书、端口台账、解析探测与 Provider 能力。

- 设计：`tsic/kos-design` 仓的 `kos-network/`（范围、契约、验收、决策记录）

## 模块布局

| 目录 | 模块 | 契约 |
|---|---|---|
| `pkg/network/declaration.go` | 页面声明：展示项、来源类型、只读原因、入口动作 | `NET-C-1`、`NET-C-2`，引用 `DASH-C-22`、`KOSD-C-53` |
| `pkg/network/endpoints.go` | 入口逐项读取：五形态、目标应用、准入冲突 | `NET-C-1`，引用 `KOSD-C-43`～`KOSD-C-45` |
| `pkg/network/certificates.go` | 证书状态、到期告警与证书身份 | `NET-C-1`、`NET-C-3`，引用 `KOSD-C-46` |
| `pkg/network/acknowledge.go` | 发起浏览器本地的告警确认 | `NET-C-3`，引用 `KOSD-C-53` |
| `pkg/network/ports.go` | 端口台账投影与冲突 | `NET-C-1`，引用 `KOSD-C-44` |
| `pkg/network/dns.go` | 读时域名解析探测 | `NET-C-1`，引用 `KOSD-C-53` |
| `pkg/network/providers.go` | 网络 Provider 能力 | `NET-C-1`，引用 `KOSD-C-47` |
| `pkg/network/actions.go` | 入口创建、修改、删除，只经控制面 API | `NET-C-2`，引用 `KOSD-C-12` |
| `pkg/network/manifest.go` | 打包清单与带摘要的工件 | `KOSD-C-17`、`KOSD-C-18`，引用 `SDK-C-3`、`SDK-C-4` |
| `tests/acceptance/<TC-id>/` | 验收用例，目录名取设计编号 | `03-acceptance.md` 的对应条目 |
| `tests/acceptance/support/` | 用例装配：控制面、页面外壳、已安装的网络包 | — |

## 页面声明

网络页是九页中的面板页，不是 `App`，也不创建 iframe（`DASH-C-22`；ADR-033：默认 UI-only、无自有后端）。
每个展示项声明来源类型与来源，按 `KOSD-C-53` 判定：

| 项 | 来源 | 只读 | 说明 |
|---|---|---|---|
| `endpoints` | 对象源：入口 | 否 | 五形态与目标应用；每行带 `resourceVersion`，修改/删除的前置条件取自这里；修改入口为 `update-endpoint` |
| `certificates` | 对象源：入口 | 是 | 证书状态，签发者取入口状态 |
| `certificate-alerts` | 计算源：证书有效期 | 是 | 到期前 14 天的告警；确认是浏览器本地写入，不是控制面写入 |
| `port-ledger` | 对象源：台账投影 | 是 | 键、冲突对象与共享介质说明 |
| `domain-resolution` | 探测源 | 是 | 读时探测结果，观测时间随读取给出 |
| `providers` | 对象源：Provider | 是 | `network.*` 能力、校验结果与条件 |
| `certificate-alert-acknowledgements` | 本地源 | 是 | 浏览器存储 `kos-network.certificate-alert-acknowledgements` |

只读项都给出原因且不提供修改入口。入口动作只有三条，都是控制面 API 的写请求：

| 动作 | 方法 | 路径 |
|---|---|---|
| `create-endpoint` | POST | `/api/v1/resources/Endpoint?namespace={namespace}` |
| `update-endpoint` | PUT | `/api/v1/resources/Endpoint/{name}?namespace={namespace}` |
| `delete-endpoint` | DELETE | `/api/v1/resources/Endpoint/{name}?namespace={namespace}` |

本仓不直接写 Ingress、Service 或证书：路由对象由入口调和器独占写（`KOSD-C-43`），页面拿不到集群凭据。

## 证书告警确认

确认状态属浏览器本地源：本仓定义记录的形状与判定规则，页面写发起浏览器的存储，不进集群对象投影、
不进偏好库（`NET-C-3`、`KOSD-C-53`）。提醒的身份是入口与证书的「签发者 + 到期时刻」；证书更换或续签
改变这个身份，所以确认过的提醒对新证书重新出现。确认从不修改入口对象或其条件。

外壳按以下约定过滤提醒，因此「确认后本浏览器不再显示」在渲染层也成立：

- 存储键 `kos-network.certificate-alert-acknowledgements`，值是 JSON 对象，键为告警的 `identity`，值为确认时刻（RFC 3339）。
- `certificate-alerts` 的每个告警带 `identity`；外壳只渲染 `identity` 未出现在本浏览器记录里的告警，另一浏览器没有该记录，提醒照常显示。
- 确认只增写本浏览器存储，不发任何控制面请求；入口对象与条件不因确认而变化。

## 打包

`kos-sdk` 的打包工具产出工件：`manifest.json`、`digest` 与作者声明的资产，摘要覆盖清单与全部资产
（`SDK-C-3`、`SDK-C-4`）。网络页是服务端投影，由控制台外壳渲染，因此包内没有 UI 资产，摘要只覆盖清单。
第一方与第三方使用同一路径，工具不读产品身份。`pkg/network.Artifact()` 是在本仓生成该工件唯一入口。

清单字段固定为内核 `KOSD-C-18` 的集合：`identifier`、`version`、`frameworkVersion`、`contributions`、
`dependencies`、`permissions`、`configuration`、`retention`。本产品的 `permissions` 声明网络页读取的
`Endpoint`、`Application`、`Provider` 三类；`retention` 声明 `pages` 随卸载删除（数据由内核对象承载，不受影响）。
安装请求以组件 `spec.package.reference` 与 `spec.package.digest` 指向该工件字节，内核校验摘要与清单后装载。

## 构建与测试

```sh
make lint        # gofmt 检查 + go vet
make test        # 单元与验收用例（需要 envtest 资产）
make acceptance  # 只跑 tests/acceptance/，-v
```

`make test` 与 `make acceptance` 会先下载并校验 envtest 资产；只想跑一个用例时：

```sh
KUBEBUILDER_ASSETS=$PWD/bin/envtest go test ./tests/acceptance/NET-TC-1/ -count=1 -v
```

当前验收用例：`NET-TC-1`（P1）、`NET-TC-73`（P0）、`NET-TC-74`（P0）、`NET-TC-75`（P1）、
`NET-TC-76`（P1）、`NET-TC-77`（P1）、`NET-TC-78`（P1）、`NET-TC-79`（P1）、`NET-TC-169`（P1）、
`NET-TC-80`（P2）、`NET-TC-170`（P2）。`tests/acceptance/install/` 覆盖本仓打包产物的装载、卸载与重装
（`KOSD-AC-72`；`KOSD-TC-155` 的用例目录属 kosd 仓）。

## 边界

- 不实现 SMB/NFS/iSCSI/WebDAV/FTP 协议栈、DNS 服务器或 CNI（`KOSD-NG-11`）。
- 不直接写 Ingress、Service 或证书，不提供绕过 kosd 的入口写路径。
- 只引用其他项目的契约条目，不复制判据。
