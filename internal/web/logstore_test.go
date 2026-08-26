package web

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestLogStoreSyncAndLimit(t *testing.T) {
	ls := NewLogStore(3)
	core := ls.Core(zap.NewNop().Core())
	logger := zap.New(core)

	for i := 0; i < 6; i++ {
		e := logger.Check(zapcore.InfoLevel, "msg")
		require.NotNil(t, e)
		e.Write()
	}

	entries := ls.List("", 10)
	assert.LessOrEqual(t, len(entries), 3, "ring buffer limité à la capacité")

	// Sync ne panique pas
	assert.NotPanics(t, func() { _ = ls.List("", 1) })
}
