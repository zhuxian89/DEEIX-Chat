package wechat

import (
	"encoding/xml"
	"errors"
	"time"

	appwechat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/wechat"
	domainwechat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/wechat"
)

type replyImage struct {
	MediaID string `xml:"MediaId"`
}

type imageReply struct {
	XMLName      xml.Name   `xml:"xml"`
	ToUserName   string     `xml:"ToUserName"`
	FromUserName string     `xml:"FromUserName"`
	CreateTime   int64      `xml:"CreateTime"`
	MsgType      string     `xml:"MsgType"`
	Image        replyImage `xml:"Image"`
}

func marshalReply(message inboundMessage, result appwechat.MessageResult) ([]byte, error) {
	switch result.ResponseType {
	case "", domainwechat.ResponseTypeText:
		return xml.Marshal(textReply{
			ToUserName: message.FromUserName, FromUserName: message.ToUserName,
			CreateTime: time.Now().Unix(), MsgType: domainwechat.ResponseTypeText,
			Content: result.Content,
		})
	case domainwechat.ResponseTypeImage:
		if result.MediaID == "" {
			return nil, errors.New("image reply requires a media ID")
		}
		return xml.Marshal(imageReply{
			ToUserName: message.FromUserName, FromUserName: message.ToUserName,
			CreateTime: time.Now().Unix(), MsgType: domainwechat.ResponseTypeImage,
			Image: replyImage{MediaID: result.MediaID},
		})
	default:
		return nil, errors.New("unsupported wechat reply type")
	}
}
