package logger

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func Test_toLevel(t *testing.T) {
	cases := map[string]zapcore.Level{
		Level_Debug: zapcore.DebugLevel,
		Level_Info:  zapcore.InfoLevel,
		Level_Warn:  zapcore.WarnLevel,
		Level_Error: zapcore.ErrorLevel,
		Level_Panic: zapcore.PanicLevel,
		Level_Fatal: zapcore.FatalLevel,
		"DEBUG":     zapcore.DebugLevel, // 大小写不敏感
		"unknown":   zapcore.InfoLevel,  // 未知取值回退 info
		"":          zapcore.InfoLevel,
	}
	for in, want := range cases {
		assert.Equal(t, want, toLevel(in), "toLevel(%q)", in)
	}
}

// newObservedHolder 构造一个与生产路径同构的 Holder：
// 同一 AtomicLevel 同时挂在 Holder 的 core 与 observer core 上，
// 验证 SetLevel 经实例真实生效。
func newObservedHolder(t *testing.T, level string) (*Holder, *observer.ObservedLogs) {
	t.Helper()
	h, err := NewStdLogger(level)
	require.NoError(t, err)
	core, logs := observer.New(h.level)
	h.zap = zap.New(core)
	return h, logs
}

func Test_holder_SetLevel(t *testing.T) {
	h, logs := newObservedHolder(t, Level_Warn)
	logs.TakeAll() // 清掉构造期可能的预置输出，从干净状态开始断言
	h.Logger().Debug("debug 应被过滤")
	h.Logger().Warn("warn 应被记录")
	require.Equal(t, 1, logs.Len())

	// 动态调级后，同一实例的 core 立即放行更低等级
	h.SetLevel(Level_Debug)
	logs.TakeAll() // 清掉调级前计数，单独验证新放行的 debug
	h.Logger().Debug("调级后 debug 应被记录")
	assert.Equal(t, 1, logs.Len(), "SetLevel 应即时生效")
}

func Test_holder_Close(t *testing.T) {
	// stdout 实例 close 为 nil：Close 空操作不报错
	hs, err := NewStdLogger(Level_Info)
	require.NoError(t, err)
	assert.NoError(t, hs.Close())

	// 文件实例：使用临时目录，避免测试产物落在包目录
	hf, err := NewFileLogger(t.TempDir()+"/curr.log", Level_Debug)
	require.NoError(t, err)
	hf.Logger().Debug("文件 debug 日志", zap.String("tt", "ok"))
	assert.NoError(t, hf.Close())
}

func Test_setGlobal_and_SetGlobalLevel(t *testing.T) {
	defer func() { // 经 SetGlobal 还原全局槽，避免污染其他测试
		h, err := NewStdLogger(Level_Info)
		require.NoError(t, err)
		SetGlobal(h)
	}()

	h, logs := newObservedHolder(t, Level_Warn)
	SetGlobal(h)

	// L() 应返回装载后的全局实例
	L().Warn("after replace")
	require.Equal(t, 1, logs.Len())

	// 全局调级经登记的持有者写回同一 core
	SetGlobalLevel(Level_Debug)
	logs.TakeAll() // 清掉调级前计数
	L().Debug("全局调级后 debug 应被记录")
	assert.Equal(t, 1, logs.Len())
}

func Test_setGlobalLevel_noRegistration_isNoop(t *testing.T) {
	// 绕过 SetGlobal 直接清空槽（仅包内测试可行的异常态），验证 L() 回退与调级空操作
	globalHolder.Store(nil)
	defer func() {
		h, err := NewStdLogger(Level_Info)
		require.NoError(t, err)
		SetGlobal(h)
	}()
	assert.NotPanics(t, func() { SetGlobalLevel(Level_Debug) })
	assert.NotNil(t, L(), "L() 在槽位被清空时仍不回 nil")
}

func Test_setGlobalLevel_onlyAffectsGlobalInstance(t *testing.T) {
	defer func() {
		h, err := NewStdLogger(Level_Info)
		require.NoError(t, err)
		SetGlobal(h)
	}()

	// 全局实例为 warn
	gh, glogs := newObservedHolder(t, Level_Warn)
	SetGlobal(gh)
	// 未经 SetGlobal 登记的独立实例，携带自己的等级句柄
	uh, ulogs := newObservedHolder(t, Level_Warn)

	SetGlobalLevel(Level_Debug)
	L().Debug("全局 debug 应被放行")
	assert.Equal(t, 1, glogs.Len())

	uh.Logger().Debug("未登记实例不受全局调级影响")
	assert.Equal(t, 0, ulogs.Len())
}

func Test_global_concurrent(t *testing.T) {
	defer func() {
		h, err := NewStdLogger(Level_Info)
		require.NoError(t, err)
		SetGlobal(h)
	}()

	h, err := NewStdLogger(Level_Info)
	require.NoError(t, err)
	SetGlobal(h)

	// 并发读 L() / 写 SetGlobal() / 调 SetGlobalLevel()，-race 下验证原子通路
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); SetGlobal(h) }()
		go func() { defer wg.Done(); _ = L().Core() }()
		go func() { defer wg.Done(); SetGlobalLevel(Level_Debug) }()
	}
	wg.Wait()
}

func Test_sysStatInfo(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	l := zap.New(core)

	SysStatInfo(l)

	all := logs.All()
	require.Len(t, all, 1)
	assert.Equal(t, "system stat metrics", all[0].Message)

	fields := all[0].ContextMap()
	assert.Equal(t, "sysmetrics", fields["type"])
	_, hasGoroutine := fields["goroutine"]
	assert.True(t, hasGoroutine, "goroutine 字段应存在")
	// mem_percent / cpu_percent 为 0~100 百分比浮点原值（采集失败时允许缺失）
	if v, ok := fields["mem_percent"]; ok {
		f, isFloat := v.(float64)
		assert.True(t, isFloat, "mem_percent 应为浮点百分比")
		assert.GreaterOrEqual(t, f, float64(0))
		assert.LessOrEqual(t, f, float64(100))
	}
	if v, ok := fields["cpu_percent"]; ok {
		f, isFloat := v.(float64)
		assert.True(t, isFloat, "cpu_percent 应为浮点百分比")
		assert.GreaterOrEqual(t, f, float64(0))
		assert.LessOrEqual(t, f, float64(100))
	}
}

func Test_defaultGlobal_available(t *testing.T) {
	// init() 承载的默认实例应在任何显式初始化前可用
	assert.NotNil(t, L())
	L().Info("默认全局实例可用")
}
