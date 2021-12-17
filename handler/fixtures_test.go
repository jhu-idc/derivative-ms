package handler

import (
	"bytes"
	"context"
	"derivative-ms/api"
	"derivative-ms/cmd"
	"derivative-ms/config"
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

	retBody = m.get.retBody
	retErr = m.get.retErr

	if retBody == nil {
		retBody = ioutil.NopCloser(&bytes.Buffer{})
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

type suite struct {
	// the context.Context provided to the Handler
	ctx ctxStruct
	// the configuration of the Handler
	configuration config.Configuration
	// a client that mocks GETs and PUTs to Drupal
	drupalClient *mockDrupal
}

type imSuite struct {
	suite
	handler *ImageMagickHandler
}

type ffmpegSuite struct {
	suite
	handler *FFMpegHandler
}

type pdf2TextSuite struct {
	suite
	handler *Pdf2TextHandler
}

type tesseractSuite struct {
	suite
	handler *TesseractHandler
}

type mutableHandler interface {
	api.Handler
	setCommandBuilder(c cmd.Builder)
	setCommandPath(cmdPath string)
}

func (s imSuite) setCommandBuilder(c cmd.Builder) {
	s.handler.CommandBuilder = c
}

func (s imSuite) setCommandPath(cmd string) {
	s.handler.CommandPath = cmd
}

func (s imSuite) Handle(ctx context.Context, t *jwt.Token, b *api.MessageBody) (context.Context, error) {
	return s.handler.Handle(ctx, t, b)
}

func (s ffmpegSuite) setCommandBuilder(c cmd.Builder) {
	s.handler.CommandBuilder = c
}

func (s ffmpegSuite) setCommandPath(cmd string) {
	s.handler.CommandPath = cmd
}

func (s ffmpegSuite) Handle(ctx context.Context, t *jwt.Token, b *api.MessageBody) (context.Context, error) {
	return s.handler.Handle(ctx, t, b)
}

func (s pdf2TextSuite) Handle(ctx context.Context, t *jwt.Token, b *api.MessageBody) (context.Context, error) {
	return s.handler.Handle(ctx, t, b)
}

func (s pdf2TextSuite) setCommandBuilder(c cmd.Builder) {
	s.handler.CommandBuilder = c
}

func (s pdf2TextSuite) setCommandPath(cmdPath string) {
	s.handler.CommandPath = cmdPath
}

func (s tesseractSuite) Handle(ctx context.Context, t *jwt.Token, b *api.MessageBody) (context.Context, error) {
	return s.handler.Handle(ctx, t, b)
}

func (s tesseractSuite) setCommandBuilder(c cmd.Builder) {
	s.handler.CommandBuilder = c
}

func (s tesseractSuite) setCommandPath(cmdPath string) {
	s.handler.CommandPath = cmdPath
}
