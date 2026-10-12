# -*- coding: utf-8 -*-
"""
cron: 18 8 * * *
new Env('移动云盘每日任务')

青龙面板脚本：移动云盘签到、任务中心、领云朵等每日任务（单账号）。

环境变量：
  CAIYUN_AUTH        必填。抓包得到的 Authorization（Basic xxx，也可以只填 xxx）
  CAIYUN_SKIP        可选。跳过的任务编码，逗号分隔，例如 fun_ai,poster_activity
  CAIYUN_ONLY        可选。只跑这些任务编码，逗号分隔
  CAIYUN_GH_PROXY    可选。下载程序用的 GitHub 加速前缀，例如 https://ghfast.top/
  CAIYUN_BIN         可选。自己编译好的程序路径，设置后不再下载
  CAIYUN_NOTIFY      可选。设为 0 关闭青龙通知；设为 fail 只在有失败时通知

任务编码列表：运行 `task caiyun_ql.py list` 查看。
"""
import json
import os
import platform
import stat
import subprocess
import sys
import time
import urllib.request

REPO = "i125pro/caiyun"
HERE = os.path.dirname(os.path.abspath(__file__))
# 放在仓库目录外，青龙更新仓库时不会被清掉
DATA = os.getenv("CAIYUN_DATA") or ("/ql/data/caiyun" if os.path.isdir("/ql/data") else os.path.join(HERE, "caiyun_data"))
ARCH = {
    "x86_64": "amd64", "amd64": "amd64",
    "aarch64": "arm64", "arm64": "arm64",
    "armv7l": "armv7", "armv7": "armv7",
}


def log(*a):
    print(*a, flush=True)


def proxied(url):
    p = os.getenv("CAIYUN_GH_PROXY", "").strip()
    if p and not p.endswith("/"):
        p += "/"
    return p + url if p else url


def http_get(url, timeout=30):
    req = urllib.request.Request(url, headers={"User-Agent": "caiyun-ql"})
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return r.read()


def latest_tag():
    try:
        data = json.loads(http_get(f"https://api.github.com/repos/{REPO}/releases/latest", 15))
        return data.get("tag_name", "")
    except Exception as e:  # 拿不到版本号就继续用本地程序
        log("检查新版本失败，继续使用本地程序：", e)
        return ""


def ensure_binary():
    custom = os.getenv("CAIYUN_BIN", "").strip()
    if custom:
        return custom
    arch = ARCH.get(platform.machine().lower())
    if not arch:
        raise SystemExit(f"❌ 不支持的 CPU 架构 {platform.machine()}，请自己编译后设置 CAIYUN_BIN")
    os.makedirs(DATA, exist_ok=True)
    path = os.path.join(DATA, f"caiyun_ql-linux-{arch}")
    tag_file = path + ".tag"
    local_tag = open(tag_file).read().strip() if os.path.exists(tag_file) else ""

    # 每天最多检查一次新版本
    need_check = not os.path.exists(path) or time.time() - os.path.getmtime(tag_file if local_tag else path) > 86400
    tag = latest_tag() if need_check else local_tag
    if os.path.exists(path) and (not tag or tag == local_tag):
        if local_tag:
            os.utime(tag_file)
        return path

    if not tag:
        raise SystemExit("❌ 无法从 GitHub 获取程序，可设置 CAIYUN_GH_PROXY 加速，或用 CAIYUN_BIN 指定本地程序")
    url = proxied(f"https://github.com/{REPO}/releases/download/{tag}/caiyun_ql-linux-{arch}")
    log(f"下载程序 {tag}（{arch}）……")
    data = http_get(url, 180)
    if len(data) < 1_000_000:
        raise SystemExit("❌ 下载的程序不完整，请稍后再试或换一个 CAIYUN_GH_PROXY")
    tmp = path + ".tmp"
    with open(tmp, "wb") as f:
        f.write(data)
    os.chmod(tmp, os.stat(tmp).st_mode | stat.S_IEXEC)
    os.replace(tmp, path)
    with open(tag_file, "w") as f:
        f.write(tag)
    return path


def notify(title, content):
    mode = os.getenv("CAIYUN_NOTIFY", "1").strip().lower()
    if mode in ("0", "false", "off", "no"):
        return
    if mode == "fail" and "❌" not in content:
        return
    for d in (HERE, os.path.dirname(HERE), "/ql/data/scripts", "/ql/scripts"):
        if d not in sys.path:
            sys.path.append(d)
    try:
        from notify import send
    except Exception:
        log("没有找到青龙的 notify.py，跳过通知")
        return
    send(title, content)


def main():
    binary = ensure_binary()
    args = [binary] + sys.argv[1:]
    env = dict(os.environ, NO_COLOR="1", CAIYUN_DATA=DATA)
    proc = subprocess.Popen(args, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, bufsize=1)
    out = []
    for line in proc.stdout:
        print(line, end="", flush=True)
        out.append(line)
    code = proc.wait()
    if sys.argv[1:]:  # list / version 等子命令不通知
        return code
    text = "".join(out)
    mark = "======== 移动云盘任务汇总 ========"
    summary = text.split(mark, 1)[1].strip() if mark in text else "".join(out[-8:]).strip()
    notify("移动云盘每日任务", summary)
    return code


if __name__ == "__main__":
    sys.exit(main())
