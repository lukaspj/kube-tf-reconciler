package runner

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExec_Error(t *testing.T) {
	t.Run("non-existing dir", func(t *testing.T) {
		tf := NewTerraform("terraform", "./non-existing")
		err := tf.Init(context.Background())
		assert.ErrorIs(t, err, ErrRunning)
	})

	t.Run("non-existing config", func(t *testing.T) {
		tf := NewTerraform("terraform", t.TempDir())
		err := tf.Apply(context.Background())
		assert.ErrorIs(t, err, ErrRunning)
	})
}

func TestExec_Success(t *testing.T) {
	t.Run("version", func(t *testing.T) {
		tf := NewTerraform("terraform", t.TempDir())
		v, err := tf.Version(context.Background())
		assert.NoError(t, err)
		assert.NotEmpty(t, v.TerraformVersion)
	})
	t.Run("init", func(t *testing.T) {
		tf := NewTerraform("terraform", t.TempDir())
		err := tf.Init(context.Background())
		assert.NoError(t, err)
	})
}
