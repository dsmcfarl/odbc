// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package odbc

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"
	"unsafe"

	"github.com/alexbrainman/odbc/api"
)

func IsError(ret api.SQLRETURN) bool {
	return !(ret == api.SQL_SUCCESS || ret == api.SQL_SUCCESS_WITH_INFO)
}

type DiagRecord struct {
	State       string
	NativeError int
	Message     string
}

func (r *DiagRecord) String() string {
	return fmt.Sprintf("{%s} %s", r.State, r.Message)
}

type Error struct {
	APIName string
	Diag    []DiagRecord
}

func (e *Error) Error() string {
	ss := make([]string, len(e.Diag))
	for i, r := range e.Diag {
		ss[i] = r.String()
	}
	return e.APIName + ": " + strings.Join(ss, "\n")
}

func NewError(apiName string, handle interface{}) error {
	h, ht, herr := ToHandleAndType(handle)
	// Extreme wierdness here: without this Sleep call, api.SQLGetDiagRec
	// returns -2 (SQL_INVALID_HANDLE) when the Sage ODBC driver returns an
	// error. It sets the msg and state even though it returns -2 though.
	// Putting a fmt.Printf("blah\n") statement also works.
	time.Sleep(1 * time.Millisecond)
	if herr != nil {
		return herr
	}
	err := &Error{APIName: apiName}
	var ne api.SQLINTEGER
	state := make([]uint16, 6)
	msg := make([]uint16, api.SQL_MAX_MESSAGE_LENGTH)
	// More wierdness here: using i := 1 and then putting api.SQLSMALLINT(i) in
	// the call to SQLGetDiagRec results in i getting set to 0 after/during the
	// call to SQLGetDiagRec. This results in an infinite loop. This seems to
	// work though.
	for i := api.SQLSMALLINT(1); ; i++ {
		ret := api.SQLGetDiagRec(ht, h, i,
			(*api.SQLWCHAR)(unsafe.Pointer(&state[0])), &ne,
			(*api.SQLWCHAR)(unsafe.Pointer(&msg[0])),
			api.SQLSMALLINT(len(msg)), nil)
		if ret == api.SQL_NO_DATA {
			break
		}
		if IsError(ret) {
			return fmt.Errorf("SQLGetDiagRec failed: ret=%d", ret)
		}
		r := DiagRecord{
			State:       api.UTF16ToString(state),
			NativeError: int(ne),
			Message:     api.UTF16ToString(msg),
		}
		if r.State == "08S01" {
			return driver.ErrBadConn
		}
		err.Diag = append(err.Diag, r)
	}
	return err
}
