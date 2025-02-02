// Copyright 2023 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package errctx

import (
	"github.com/pingcap/errors"
	"github.com/pingcap/tidb/pkg/errno"
	"github.com/pingcap/tidb/pkg/util/intest"
)

// Level defines the behavior for each error
type Level uint8

const (
	// LevelError means the error will be returned
	LevelError Level = iota
	// LevelWarn means it will be regarded as a warning
	LevelWarn
	// LevelIgnore means the error will be ignored
	LevelIgnore
)

// Context defines how to handle an error
type Context struct {
	levelMap        [errGroupCount]Level
	appendWarningFn func(err error)
}

// WithStrictErrGroupLevel makes the context to return the error directly for any kinds of errors.
func (ctx *Context) WithStrictErrGroupLevel() Context {
	newCtx := Context{
		appendWarningFn: ctx.appendWarningFn,
	}

	return newCtx
}

// WithErrGroupLevel sets a `Level` for an `ErrGroup`
func (ctx *Context) WithErrGroupLevel(eg ErrGroup, l Level) Context {
	newCtx := Context{
		levelMap:        ctx.levelMap,
		appendWarningFn: ctx.appendWarningFn,
	}
	newCtx.levelMap[eg] = l

	return newCtx
}

// appendWarning appends the error to warning. If the inner `appendWarningFn` is nil, do nothing.
func (ctx *Context) appendWarning(err error) {
	intest.Assert(ctx.appendWarningFn != nil)
	if fn := ctx.appendWarningFn; fn != nil {
		// appendWarningFn should always not be nil, check fn != nil here to just make code safe.
		fn(err)
	}
}

// HandleError handles the error according to the context. See the comment of `HandleErrorWithAlias` for detailed logic.
func (ctx *Context) HandleError(err error) error {
	return ctx.HandleErrorWithAlias(err, err, err)
}

// HandleErrorWithAlias handles the error according to the context.
//  1. If the `internalErr` is not `"pingcap/errors".Error`, or the error code is not defined in the `errGroupMap`, or the error
//     level is set to `LevelError`(0), the `err` will be returned directly.
//  2. If the error level is set to `LevelWarn`, the `warnErr` will be appended as a warning.
//  3. If the error level is set to `LevelIgnore`, this function will return a `nil`.
//
// In most cases, these three should be the same. If there are many different kinds of error internally, but they are expected
// to give the same error to users, the `err` can be different form `internalErr`. Also, if the warning is expected to be
// different from the initial error, you can also use the `warnErr` argument.
//
// TODO: is it good to give an error code for internal only errors? Or should we use another way to distinguish different
// group of errors?
// TODO: both `types.Context` and `errctx.Context` can handle truncate error now. Refractor them.
func (ctx *Context) HandleErrorWithAlias(internalErr error, err error, warnErr error) error {
	if internalErr == nil {
		return nil
	}

	internalErr = errors.Cause(internalErr)

	e, ok := internalErr.(*errors.Error)
	if !ok {
		return err
	}

	eg, ok := errGroupMap[e.Code()]
	if !ok {
		return err
	}

	switch ctx.levelMap[eg] {
	case LevelError:
		return err
	case LevelWarn:
		ctx.appendWarning(warnErr)
	case LevelIgnore:
	}

	return nil
}

// NewContext creates an error context to handle the errors and warnings
func NewContext(appendWarningFn func(err error)) Context {
	intest.Assert(appendWarningFn != nil)
	return Context{
		appendWarningFn: appendWarningFn,
	}
}

// StrictNoWarningContext returns all errors directly, and ignore all errors
var StrictNoWarningContext = NewContext(func(_ error) {
	// the error is ignored
})

var errGroupMap = make(map[errors.ErrCode]ErrGroup)

// ErrGroup groups the error according to the behavior of handling errors
type ErrGroup int

const (
	// ErrGroupTruncate is the group of truncated errors
	ErrGroupTruncate ErrGroup = iota
	// ErrGroupOverflow is the group of overflow errors
	ErrGroupOverflow

	// errGroupCount is the count of all `ErrGroup`. Please leave it at the end of the list.
	errGroupCount
)

func init() {
	truncateErrCodes := []errors.ErrCode{
		errno.ErrTruncatedWrongValue,
		errno.ErrDataTooLong,
		errno.ErrTruncatedWrongValueForField,
		errno.ErrWarnDataOutOfRange,
		errno.ErrDataOutOfRange,
		errno.ErrBadNumber,
		errno.ErrWrongValueForType,
		errno.ErrDatetimeFunctionOverflow,
		errno.WarnDataTruncated,
		errno.ErrIncorrectDatetimeValue,
	}
	for _, errCode := range truncateErrCodes {
		errGroupMap[errCode] = ErrGroupTruncate
	}

	errGroupMap[errno.ErrDataOutOfRange] = ErrGroupOverflow
}
