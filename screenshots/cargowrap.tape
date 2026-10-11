Output screenshots/cargowrap.gif

# 设置终端外观
Set FontSize 20
Set Width 1200
Set Height 390
Set Padding 6

# cargowrap 把 cargo build --release 的 stderr 逐行喂给 spinner，因此必须在 Rust 工程
# 里跑；而 go run 会把 cwd 锁在 Go 模块内，所以先把示例编成 exe，再到 %TEMP% 里现建
# 一个带依赖的空 crate（--offline，本地 registry 已缓存），让编译过程够长，看得清 spinner。
Type "go build -o %TEMP%\cargowrap.exe github.com/quant1x/mpb/examples/cargowrap"
Enter
Sleep 6s
Type "cd /d %TEMP% & rd /s /q vhs-cargowrap-demo & cargo new --bin vhs-cargowrap-demo"
Enter
Sleep 4s
Type "cd vhs-cargowrap-demo & cargo add serde --features derive --offline"
Enter
Sleep 6s
Type "%TEMP%\cargowrap.exe"
Enter
Sleep 20s
