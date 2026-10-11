<#
.SYNOPSIS
    批量用 VHS 重录 screenshots 下的 GIF。

.DESCRIPTION
    遍历 screenshots/*.tape 逐条调用 vhs（跳过 _ 开头的探针 tape）。
    必须从仓库根目录跑 —— tape 里的 Output 路径相对 cwd 解析；
    脚本自己会切到仓库根目录，所以从任何地方调用都行。

    用法：
        ./scripts/generate.ps1                 # 重录全部
        ./scripts/generate.ps1 single log      # 只重录指定的几条（基名，不带扩展名）
        $env:VHS='c:\runtime\work\bin\vhs.exe'; ./scripts/generate.ps1

    失败时列出失败的 tape 并以退出码 1 结束。
#>

$ErrorActionPreference = 'Stop'

# 切到仓库根目录：tape 里的 Output 相对 cwd 解析
Set-Location (Split-Path -Parent $PSScriptRoot)

$Vhs = 'vhs'
if ($env:VHS) { $Vhs = $env:VHS }

$cmd = Get-Command $Vhs -ErrorAction SilentlyContinue
if (-not $cmd) {
    throw "找不到 vhs（$Vhs）。先装 VHS，或用 `$env:VHS 指定路径。"
}

$names = @($args)
if ($names.Count -gt 0) {
    $tapes = @($names | ForEach-Object { 'screenshots/' + $_ + '.tape' })
}
else {
    $tapes = @(
        Get-ChildItem -Path screenshots -Filter '*.tape' |
            Where-Object { $_.Name -notlike '_*' } |
            Sort-Object Name |
            ForEach-Object { $_.FullName }
    )
}

if ($tapes.Count -eq 0) {
    throw '没有可录的 tape。'
}

Write-Host ('vhs: ' + $cmd.Source)
Write-Host ('共 ' + $tapes.Count + ' 条，开始录制...')
Write-Host ''

$ok = 0
$failed = @()
$stopwatch = [System.Diagnostics.Stopwatch]::StartNew()

foreach ($tape in $tapes) {
    $base = [System.IO.Path]::GetFileNameWithoutExtension($tape)

    if (-not (Test-Path $tape)) {
        Write-Host ('跳过: ' + $tape + '（不存在）')
        $failed += ($base + '（tape 不存在）')
        continue
    }

    $gif = 'screenshots/' + $base + '.gif'
    $t0 = Get-Date
    & $Vhs $tape -q 2>&1 | Out-Null
    $secs = [math]::Round(((Get-Date) - $t0).TotalSeconds, 1)

    if ($LASTEXITCODE -eq 0 -and (Test-Path $gif)) {
        $kb = [math]::Round((Get-Item $gif).Length / 1KB)
        Write-Host ('{0,-20} {1,6} KB  {2,5}s' -f $base, $kb, $secs)
        $ok++
    }
    else {
        Write-Host ('{0,-20} 失败（vhs 退出码 {1}）' -f $base, $LASTEXITCODE)
        $failed += $base
    }
}

Write-Host ''
$stopwatch.Stop()
Write-Host ('完成: ' + $ok + '/' + $tapes.Count + ' 条，用时 ' + [math]::Round($stopwatch.Elapsed.TotalSeconds) + 's')

$total = (Get-ChildItem -Path screenshots -Filter '*.gif' |
    Where-Object { $_.Name -notlike '_*' } |
    Measure-Object -Property Length -Sum).Sum
Write-Host ('GIF 合计: ' + [math]::Round($total / 1MB, 1) + ' MB')

if ($failed.Count -gt 0) {
    Write-Host ('失败: ' + ($failed -join ', '))
    exit 1
}
