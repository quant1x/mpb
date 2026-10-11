#!/usr/bin/env bash
#
# 批量用 VHS 重录 screenshots 下的 GIF。
#
# 遍历 screenshots/*.tape 逐条调用 vhs（跳过 _ 开头的探针 tape）。
# 必须从仓库根目录跑 —— tape 里的 Output 路径相对 cwd 解析；
# 脚本自己会切到仓库根目录，所以从任何地方调用都行。
#
# 用法：
#   ./scripts/generate.sh              # 重录全部
#   ./scripts/generate.sh single log   # 只重录指定的几条（基名，不带扩展名）
#   VHS=/path/to/vhs ./scripts/generate.sh
#
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

VHS=${VHS:-vhs}
if ! command -v "$VHS" >/dev/null 2>&1; then
    echo "找不到 vhs（$VHS）。先装 VHS，或用 VHS=... 指定路径。" >&2
    exit 1
fi

if [ $# -gt 0 ]; then
    tapes=()
    for n in "$@"; do
        tapes+=("screenshots/$n.tape")
    done
else
    tapes=()
    while IFS= read -r f; do
        [ -n "$f" ] && tapes+=("$f")
    done < <(find screenshots -maxdepth 1 -name '*.tape' ! -name '_*' | sort)
fi

if [ ${#tapes[@]} -eq 0 ]; then
    echo "没有可录的 tape。" >&2
    exit 1
fi

echo "vhs: $(command -v "$VHS")"
echo "共 ${#tapes[@]} 条，开始录制..."
echo

ok=0
failed=()
start=$(date +%s)

human_kb() { # 字节 -> KB，POSIX 友好的 du 输出
    du -k "$1" 2>/dev/null | awk '{print $1}'
}

for tape in "${tapes[@]}"; do
    base=${tape%.tape}
    base=${base#screenshots/}
    if [ ! -f "$tape" ]; then
        echo "跳过：$tape（不存在）"
        failed+=("$base（tape 不存在）")
        continue
    fi

    gif="screenshots/$base.gif"
    before=0
    [ -f "$gif" ] && before=$(human_kb "$gif")

    t0=$(date +%s)
    "$VHS" "$tape" -q >/dev/null 2>&1
    rc=$?
    secs=$(( $(date +%s) - t0 ))

    if [ $rc -eq 0 ] && [ -f "$gif" ]; then
        size=$(human_kb "$gif")
        printf '%-20s %6s KB  %5ss\n' "$base" "$size" "$secs"
        ok=$((ok + 1))
    else
        printf '%-20s 失败（vhs 退出码 %s）\n' "$base" "$rc"
        failed+=("$base")
    fi
done

echo
echo "完成：$ok/${#tapes[@]} 条，用时 $(( $(date +%s) - start ))s"
echo "GIF 合计：$(du -ch screenshots/*.gif 2>/dev/null | awk '/total/ {print $1}')"

if [ ${#failed[@]} -gt 0 ]; then
    echo "失败：${failed[*]}"
    exit 1
fi
