package utils_test

import (
	"errors"
	"testing"

	"github.com/mhrabovcin/troubleshoot-live/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestMaxErrorString(t *testing.T) {
	err := errors.New("foo")
	assert.Equal(t, utils.MaxErrorString(err, 100), err)
	assert.NotEqual(t, utils.MaxErrorString(err, 1), err)
	assert.EqualError(t, utils.MaxErrorString(err, 1), "f")
}
