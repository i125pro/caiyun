# 青龙面板使用说明

这个分支在原项目基础上增加了青龙脚本 `caiyun_ql.py`。它不需要 MySQL、Redis 和网页后台，青龙每天定时跑一次单账号的每日任务，跑完发一条青龙通知。

## 1. 拉取脚本

青龙「订阅管理」→「新建订阅」：

| 项 | 填写 |
| --- | --- |
| 链接 | `https://github.com/i125pro/caiyun.git` |
| 分支 | `master` |
| 白名单 | `caiyun_ql` |
| 定时 | 按需，例如 `0 0 * * *`（每天拉一次更新） |

或者在旧版青龙里用命令：

```bash
ql repo https://github.com/i125pro/caiyun.git "caiyun_ql" "" "" "master"
```

拉取后会自动生成定时任务「移动云盘每日任务」，默认每天 08:18 运行。

## 2. 添加环境变量

| 变量 | 必填 | 说明 |
| --- | --- | --- |
| `CAIYUN_AUTH` | 是 | 移动云盘 App 请求头里的 `Authorization`，形如 `Basic xxxx`，只填 `xxxx` 也行 |
| `CAIYUN_SKIP` | 否 | 不想跑的任务编码，逗号分隔，例如 `fun_ai,poster_activity` |
| `CAIYUN_ONLY` | 否 | 只跑这些任务编码，逗号分隔 |
| `CAIYUN_NOTIFY` | 否 | `0` 关闭通知，`fail` 只在有任务失败时通知，默认每次都通知 |
| `CAIYUN_GH_PROXY` | 否 | 国内机器下载程序慢时填加速前缀，例如 `https://ghfast.top/` |
| `CAIYUN_BIN` | 否 | 自己编译的程序路径，填了就不再下载 |

`Authorization` 获取方法：手机抓包打开移动云盘 App，找发往 `m.mcloud.139.com` 或 `caiyun.feixin.10086.cn` 的请求，复制请求头里的 `Authorization`。

通知走青龙自带的 `notify.py`，在青龙「系统设置 → 通知设置」或 `config.sh` 里配置推送方式即可。

## 3. 运行逻辑

- 第一次运行会从本仓库 Releases 下载对应架构的程序（amd64 / arm64 / armv7），放在 `/ql/data/caiyun`，之后每天最多检查一次新版本。
- 运行前先检查 `Authorization` 是否有效，失效会直接提示重新抓包，不会白跑一遍。
- `Authorization` 剩不到 5 天时会自动续期，新值保存在 `/ql/data/caiyun`，环境变量不用改；续期失败才需要重新抓包。
- 任务顺序和网页版一致：签到、备份翻倍、云朵翻倍、微信签到/抽奖、任务中心、领取云朵、各类活动、今日云朵统计、收尾清理。
- 只跑单账号。需要多个账号互相配合的任务（139 邮箱互发、活动互助）和抢兑暂不执行，见 [TODO](./TODO.md)。

## 4. 任务编码

| 编码 | 任务 |
| --- | --- |
| `signin` | 每日签到 |
| `task_expansion_reward` | 备份翻倍奖励 |
| `cloud_multiple` | 云朵翻倍 |
| `wechat` | 微信签到 |
| `wxdraw` | 微信抽奖 |
| `tasklist` | 任务中心巡检 |
| `invitefriends` | 邀请好友看电影 |
| `receive` | 领取云朵 |
| `notice_switch` | 通知开关同步 |
| `messagepush` | 消息推送奖励 |
| `revivalreward` | 复活卡奖励 |
| `backupgift` | 备份礼包 |
| `token_pk` | 算力大作战 |
| `make_wish` | 许愿赢好礼 |
| `fun_ai` | 趣玩AI抽奖 |
| `poster_activity` | 校园海报活动 |
| `hidden_rewards` | 隐藏活动奖励 |
| `prize_center` | 领奖专区盘点 |
| `student_perks` | 学生认证福利 |
| `todaycloud` | 今日云朵统计 |
| `after_task` | 收尾清理 |

## 5. 不用青龙也能跑

```bash
cd backend
CGO_ENABLED=0 go build -o caiyun_ql ./cmd/ql
CAIYUN_AUTH='Basic xxxx' ./caiyun_ql        # 跑一次
./caiyun_ql list                            # 查看任务编码
```

## 6. 常见问题

- **提示 Authorization 已失效**：重新抓包，更新 `CAIYUN_AUTH`。
- **下载程序失败**：设置 `CAIYUN_GH_PROXY`，或者手动下载 Releases 里的程序，用 `CAIYUN_BIN` 指定路径。
- **某个活动一直失败**：活动可能已下线或账号没资格，用 `CAIYUN_SKIP` 跳过它。
- **领奖专区、学生福利提示需要短信验证码**：脚本模式无法输入验证码，只做查询，需要领取请到 App 操作。
