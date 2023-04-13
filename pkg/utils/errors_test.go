package utils_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mhrabovcin/troubleshoot-live/pkg/utils"
)

func TestMaxErrorString(t *testing.T) {
	err := errors.New("foo")
	assert.Equal(t, utils.MaxErrorString(err, 100), err)
	assert.NotEqual(t, utils.MaxErrorString(err, 1), err)
	assert.EqualError(t, utils.MaxErrorString(err, 1), "f")
}
