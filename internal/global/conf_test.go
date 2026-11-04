package global

import (
	"flag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigFileAndFlags(t *testing.T) {
	old, oldParam, oldFlags, oldArgs := AppConf, Param, flag.CommandLine, os.Args
	defer func() { AppConf, Param, flag.CommandLine, os.Args = old, oldParam, oldFlags, oldArgs }()
	AppConf = viper.New()
	config := filepath.Join(t.TempDir(), "app.yaml")
	require.NoError(t, os.WriteFile(config, []byte("env: test\ndebug: false\n"), 0600))
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	os.Args = []string{"test", "-config", config}
	InitFlag()
	require.True(t, Flag())
	require.Equal(t, config, Param.C)
	MustInitAppConf(Param.C)
	require.Equal(t, "test", Env())
	require.False(t, IsDebug())
	require.NotEmpty(t, HostName())
	require.Equal(t, HostName(), HostName())
	require.Panics(t, func() { MustInitAppConf(filepath.Join(t.TempDir(), "missing.yaml")) })
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	os.Args = []string{"test", "-help"}
	InitFlag()
	require.False(t, Flag())
}
