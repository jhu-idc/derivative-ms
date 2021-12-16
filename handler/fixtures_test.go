package handler

import (
	"bytes"
	"context"
	"derivative-ms/api"
	"derivative-ms/drupal/request"
	"github.com/cristalhq/jwt/v4"
	"io"
	"io/ioutil"
	"os/exec"
)

type mockDrupal struct {
	put struct {
		uri     string
		reqCtx  request.Context
		body    []byte
		readErr error

		retCode int
		retErr  error
	}

	get struct {
		uri    string
		reqCtx request.Context
		body   io.ReadCloser

		retCode int
		retErr  error
		retBody io.ReadCloser
	}
}

func (m *mockDrupal) Put(reqCtx request.Context, uri string, body io.ReadCloser) (retCode int, retErr error) {
	m.put.reqCtx = reqCtx
	m.put.uri = uri
	m.put.body, m.put.readErr = ioutil.ReadAll(body)

	if m.put.retCode == 0 {
		retCode = 200
	}

	if m.put.retErr != nil {
		retErr = m.put.retErr
	}

	return
}

func (m *mockDrupal) Get(reqCtx request.Context, uri string) (retBody io.ReadCloser, retErr error) {
	m.get.reqCtx = reqCtx
	m.get.uri = uri

	if m.get.retBody == nil {
		retBody = ioutil.NopCloser(&bytes.Buffer{})
	}
	if m.get.retErr != nil {
		retErr = m.get.retErr
	}

	return
}

type mockCmd struct {
	cmd *exec.Cmd
}

func (m *mockCmd) Build(commandPath string, token *jwt.Token, body *api.MessageBody) (*exec.Cmd, error) {
	return m.cmd, nil
}

type testContext interface {
	context.Context
	withValue(key string, value interface{}) testContext
}

type ctxStruct struct {
	context.Context
	ctx context.Context
}

func newContext(messageId, queueDestination string, messageBody api.MessageBody) ctxStruct {
	ctx := ctxStruct{ctx: context.Background()}
	ctx.
		withValue(api.MsgId, messageId).
		withValue(api.MsgDestination, queueDestination).
		withValue(api.MsgBody, messageBody)
	return ctx
}

func (testContext *ctxStruct) withValue(key string, value interface{}) testContext {
	testContext.ctx = context.WithValue(testContext.ctx, key, value)
	return testContext
}
