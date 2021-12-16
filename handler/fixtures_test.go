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
}

func (m mockDrupal) Put(reqCtx request.Context, uri string, body io.ReadCloser) (int, error) {
	return 200, nil
}

func (m mockDrupal) Get(reqCtx request.Context, uri string) (io.ReadCloser, error) {
	return ioutil.NopCloser(&bytes.Buffer{}), nil
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
