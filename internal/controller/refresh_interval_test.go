package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRefreshIntervalFor(t *testing.T) {
	t.Run("defaults when unset", func(t *testing.T) {
		ws := newWs("interval-default", "some-source")
		assert.Equal(t, nextRefreshInterval, refreshIntervalFor(ws))
	})

	t.Run("uses spec value", func(t *testing.T) {
		ws := newWs("interval-custom", "some-source")
		ws.Spec.RefreshInterval = "1h30m"
		assert.Equal(t, 90*time.Minute, refreshIntervalFor(ws))
	})

	t.Run("clamps values below minimum", func(t *testing.T) {
		ws := newWs("interval-tiny", "some-source")
		ws.Spec.RefreshInterval = "10s"
		assert.Equal(t, minRefreshInterval, refreshIntervalFor(ws))
	})

	t.Run("falls back to default when invalid", func(t *testing.T) {
		ws := newWs("interval-invalid", "some-source")
		ws.Spec.RefreshInterval = "not-a-duration"
		assert.Equal(t, nextRefreshInterval, refreshIntervalFor(ws))
	})
}
