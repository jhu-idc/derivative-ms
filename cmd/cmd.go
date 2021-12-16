package cmd

import (
	"derivative-ms/api"
	"fmt"
	"github.com/cristalhq/jwt/v4"
	"os/exec"
	"strings"
)

type Builder interface {
	Build(commandPath string, token *jwt.Token, body *api.MessageBody) (*exec.Cmd, error)
}

type ImageMagick struct {
}

type FFMpeg struct {
	AcceptedFormatsMap map[string]string
}

type Tesseract struct {
}

type Pdf2Text struct {
}

func (i ImageMagick) Build(commandPath string, token *jwt.Token, body *api.MessageBody) (*exec.Cmd, error) {
	var cmdArgs []string
	cmdArgs = append(cmdArgs, commandPath)
	cmdArgs = append(cmdArgs, "-")
	// "-thumbnail 100x100", or ""
	if trimmedArgs := strings.TrimSpace(body.Attachment.Content.Args); len(trimmedArgs) > 0 {
		for _, addlArg := range strings.Split(trimmedArgs, " ") {
			cmdArgs = append(cmdArgs, addlArg)
		}
	}
	convertFormat := body.Attachment.Content.MimeType[strings.LastIndex(body.Attachment.Content.MimeType, "/")+1:]
	cmdArgs = append(cmdArgs, fmt.Sprintf("%s:-", convertFormat))
	return &exec.Cmd{
		Path: commandPath,
		Args: cmdArgs,
	}, nil
}

func (f FFMpeg) Build(commandPath string, token *jwt.Token, body *api.MessageBody) (*exec.Cmd, error) {
	const mp4Args = "-vcodec libx264 -preset medium -acodec aac -strict -2 -ab 128k -ac 2 -async 1 -movflags frag_keyframe+empty_moov"

	// Map the requested IANA media type to a supported FFmpeg output format
	var outputFormat string
	if format, ok := f.AcceptedFormatsMap[body.Attachment.Content.MimeType]; !ok {
		return nil, fmt.Errorf("cmd: ffmpeg does not support mime type '%s'", body.Attachment.Content.MimeType)
	} else {
		outputFormat = format
	}

	// Apply special arguments for the mp4 output format
	if "mp4" == outputFormat {
		if len(strings.TrimSpace(body.Attachment.Content.Args)) > 0 {
			body.Attachment.Content.Args = fmt.Sprintf("%s %s", strings.TrimSpace(body.Attachment.Content.Args), mp4Args)
		} else {
			body.Attachment.Content.Args = mp4Args
		}
	}

	var cmdArgs []string
	cmdArgs = append(cmdArgs, commandPath)
	//cmdArgs = append(cmdArgs, "-loglevel", "debug")
	cmdArgs = append(cmdArgs, "-headers", fmt.Sprintf("Authorization: %s", asBearer(token)))
	cmdArgs = append(cmdArgs, "-i", body.Attachment.Content.SourceUri)
	if trimmedArgs := strings.TrimSpace(body.Attachment.Content.Args); len(trimmedArgs) > 0 {
		for _, addlArg := range strings.Split(trimmedArgs, " ") {
			cmdArgs = append(cmdArgs, addlArg)
		}
	}
	cmdArgs = append(cmdArgs, "-f", outputFormat)
	cmdArgs = append(cmdArgs, "-")
	return &exec.Cmd{
		Path: commandPath,
		Args: cmdArgs,
	}, nil
}

func (t Tesseract) Build(commandPath string, token *jwt.Token, body *api.MessageBody) (*exec.Cmd, error) {
	var cmdArgs []string
	cmdArgs = append(cmdArgs, commandPath)
	cmdArgs = append(cmdArgs, "stdin", "stdout")
	if trimmedArgs := strings.TrimSpace(body.Attachment.Content.Args); len(trimmedArgs) > 0 {
		for _, addlArg := range strings.Split(trimmedArgs, " ") {
			cmdArgs = append(cmdArgs, addlArg)
		}
	}
	return &exec.Cmd{
		Path: commandPath,
		Args: cmdArgs,
	}, nil
}

func (p Pdf2Text) Build(commandPath string, token *jwt.Token, body *api.MessageBody) (*exec.Cmd, error) {
	var cmdArgs []string
	cmdArgs = append(cmdArgs, commandPath)
	if trimmedArgs := strings.TrimSpace(body.Attachment.Content.Args); len(trimmedArgs) > 0 {
		for _, addlArg := range strings.Split(trimmedArgs, " ") {
			cmdArgs = append(cmdArgs, addlArg)
		}
	}
	cmdArgs = append(cmdArgs, "-", "-")
	return &exec.Cmd{
		Path: commandPath,
		Args: cmdArgs,
	}, nil
}

func asBearer(token *jwt.Token) string {
	return fmt.Sprintf("Bearer %s", token)
}
