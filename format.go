package mpb

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// 以下实现基于 src/format.rs 的逐行翻译，注释中文以便与原始 Rust 注释对应

// 时间单位常量
var (
	SECOND = time.Second
	MINUTE = time.Minute
	HOUR   = time.Hour
	DAY    = 24 * HOUR
	WEEK   = 7 * DAY
	YEAR   = 365 * DAY
)

// FormattedDuration: 包装 time.Duration 用于基本人类可读格式化
type FormattedDuration time.Duration

func (d FormattedDuration) String() string {
	td := time.Duration(d)
	t := int64(td / time.Second)
	seconds := t % 60
	t /= 60
	minutes := t % 60
	t /= 60
	hours := t % 24
	t /= 24
	if t > 0 {
		days := t
		return fmt.Sprintf("%dd %02d:%02d:%02d", days, hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

// HumanDuration: 包装 time.Duration 用于更直观的人类可读显示
type HumanDuration time.Duration

type unitDef struct {
	dur  time.Duration
	name string
	alt  string
}

var units = []unitDef{
	{YEAR, "year", "y"},
	{WEEK, "week", "w"},
	{DAY, "day", "d"},
	{HOUR, "hour", "h"},
	{MINUTE, "minute", "m"},
	{SECOND, "second", "s"},
}

func (d HumanDuration) String() string {
	return humanDurationFmt(time.Duration(d), false)
}

// 支持 alternate (#) 的短格式
func humanDurationFmt(d time.Duration, altFlag bool) string {
	// 选择单位索引，遵循原始 Rust 的判定逻辑
	idx := 0
	for i := range units {
		idx = i
		if i+1 < len(units) {
			cur := units[i].dur
			next := units[i+1].dur
			if d+next/2 >= cur+cur/2 {
				break
			}
		}
	}

	unit := units[idx]
	// 使用四舍五入到最近的单位数
	t := int(math.Round(d.Seconds() / unit.dur.Seconds()))
	if idx < len(units)-1 {
		if t < 2 {
			t = 2
		}
	}

	if altFlag {
		return fmt.Sprintf("%d%s", t, unit.alt)
	}
	if t == 1 {
		return fmt.Sprintf("%d %s", t, unit.name)
	}
	return fmt.Sprintf("%d %ss", t, unit.name)
}

// 为 fmt.Formatter 的兼容性提供支持（便于处理 %#v 等）
func (d HumanDuration) Format(s fmt.State, verb rune) {
	alt := s.Flag('#')
	out := humanDurationFmt(time.Duration(d), alt)
	ioWriteString(s, out)
}

// Helper: 将字符串写入 fmt.State（避免直接依赖 io.WriteString）
func ioWriteString(s fmt.State, str string) {
	if w, ok := s.(interface{ Write([]byte) (int, error) }); ok {
		w.Write([]byte(str))
	} else {
		fmt.Fprint(s, str)
	}
}

// HumanBytes: 把字节数格式化为人类可读（使用二进制前缀，KiB 等）
type HumanBytes uint64

func (b HumanBytes) String() string {
	return binaryPrefix(uint64(b), true)
}

// DecimalBytes: 使用十进制 SI 前缀 (kB, MB)
type DecimalBytes uint64

func (b DecimalBytes) String() string {
	return decimalPrefix(uint64(b))
}

// BinaryBytes: 使用二进制前缀显示 (KiB, MiB)
type BinaryBytes uint64

func (b BinaryBytes) String() string {
	return binaryPrefix(uint64(b), false)
}

func decimalPrefix(n uint64) string {
	if n < 1000 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"kB", "MB", "GB", "TB", "PB"}
	f := float64(n)
	for i, u := range units {
		div := math.Pow(1000, float64(i+1))
		if f < div*1000 || i == len(units)-1 {
			val := f / div
			return fmt.Sprintf("%.2f %s", val, u)
		}
	}
	return fmt.Sprintf("%d B", n)
}

func binaryPrefix(n uint64, useSiSuffix bool) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	var units []string
	if useSiSuffix {
		units = []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	} else {
		units = []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	}
	f := float64(n)
	for i, u := range units {
		div := math.Pow(1024, float64(i+1))
		if f < div*1024 || i == len(units)-1 {
			val := f / div
			return fmt.Sprintf("%.2f %s", val, u)
		}
	}
	return fmt.Sprintf("%d B", n)
}

// HumanCount: 使用千位分隔符格式化整数
type HumanCount uint64

func (h HumanCount) String() string {
	s := strconv.FormatUint(uint64(h), 10)
	var b strings.Builder
	l := len(s)
	for i, ch := range s {
		pos := l - i - 1
		b.WriteRune(ch)
		if pos > 0 && pos%3 == 0 {
			b.WriteRune(',')
		}
	}
	return b.String()
}

// HumanFloatCount: 支持带小数和千位分隔符的浮点数，尊重格式化精度
type HumanFloatCount float64

// 实现 fmt.Formatter 以便读取精度信息
func (h HumanFloatCount) Format(s fmt.State, verb rune) {
	prec, ok := s.Precision()
	if !ok {
		prec = 4
	}
	// 使用指定精度格式化
	fmtStr := "%.*f"
	num := fmt.Sprintf(fmtStr, prec, float64(h))
	intPart := num
	fracPart := ""
	if idx := strings.IndexByte(num, '.'); idx >= 0 {
		intPart = num[:idx]
		fracPart = strings.TrimRight(num[idx+1:], "0")
	}
	// 整数部分插入千位分隔符
	var b strings.Builder
	l := len(intPart)
	for i, ch := range intPart {
		pos := l - i - 1
		b.WriteRune(ch)
		if pos > 0 && pos%3 == 0 {
			b.WriteRune(',')
		}
	}
	if fracPart != "" {
		b.WriteByte('.')
		b.WriteString(fracPart)
	}
	ioWriteString(s, b.String())
}
