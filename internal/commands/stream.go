package commands

import (
	"fmt"
	"time"

	"EverythingSuckz/fsb/config"
	"EverythingSuckz/fsb/internal/control"
	"EverythingSuckz/fsb/internal/utils"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/storage"
	"github.com/celestix/gotgproto/types"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"
)

func (m *command) LoadStream(dispatcher dispatcher.Dispatcher) {
	log := m.log.Named("gdplayer")
	defer log.Sugar().Info("Loaded GDPlayer media handler")
	dispatcher.AddHandler(handlers.NewMessage(nil, sendLink))
}

func supportedMediaFilter(m *types.Message) (bool, error) {
	if m.Media == nil {
		return false, dispatcher.EndGroups
	}
	switch m.Media.(type) {
	case *tg.MessageMediaDocument, *tg.MessageMediaPhoto:
		return true, nil
	case tg.MessageMediaClass:
		return false, dispatcher.EndGroups
	default:
		return false, nil
	}
}

// sendLink forwards supported user media to the configured Telegram log channel
// and asks GDPlayer to register the resulting metadata. Render handles only the
// lightweight control request; all media bytes are later fetched by GDPlayer.
func sendLink(ctx *ext.Context, u *ext.Update) error {
	chatID := u.EffectiveChat().GetID()
	peerChatID := ctx.PeerStorage.GetPeerById(chatID)
	if peerChatID.Type != int(storage.TypeUser) {
		return dispatcher.EndGroups
	}
	if len(config.ValueOf.AllowedUsers) != 0 && !utils.Contains(config.ValueOf.AllowedUsers, chatID) {
		ctx.Reply(u, ext.ReplyTextString("You are not allowed to use this bot."), nil)
		return dispatcher.EndGroups
	}
	if config.ValueOf.GDPlayerControlURL == "" || config.ValueOf.GDPlayerSharedSecret == "" {
		ctx.Reply(u, ext.ReplyTextString("GDPlayer integration is not configured. No stream link was created."), nil)
		return dispatcher.EndGroups
	}
	supported, err := supportedMediaFilter(u.EffectiveMessage)
	if err != nil {
		return err
	}
	if !supported {
		ctx.Reply(u, ext.ReplyTextString("Sorry, this message type is unsupported."), nil)
		return dispatcher.EndGroups
	}

	update, err := utils.ForwardMessages(ctx, chatID, config.ValueOf.LogChannelID, u.EffectiveMessage.ID)
	if err != nil {
		utils.Logger.Sugar().Error(err)
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("Error - %s", err.Error())), nil)
		return dispatcher.EndGroups
	}
	if len(update.Updates) < 2 {
		ctx.Reply(u, ext.ReplyTextString("Error - unexpected Telegram forwarding response"), nil)
		return dispatcher.EndGroups
	}
	messageIDUpdate, ok := update.Updates[0].(*tg.UpdateMessageID)
	if !ok {
		ctx.Reply(u, ext.ReplyTextString("Error - unexpected Telegram message ID response"), nil)
		return dispatcher.EndGroups
	}
	newMessage, ok := update.Updates[1].(*tg.UpdateNewChannelMessage)
	if !ok {
		ctx.Reply(u, ext.ReplyTextString("Error - unexpected Telegram channel response"), nil)
		return dispatcher.EndGroups
	}
	message, ok := newMessage.Message.(*tg.Message)
	if !ok {
		ctx.Reply(u, ext.ReplyTextString("Error - unexpected Telegram message type"), nil)
		return dispatcher.EndGroups
	}
	file, err := utils.FileFromMedia(message.Media)
	if err != nil {
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("Error - %s", err.Error())), nil)
		return dispatcher.EndGroups
	}
	if file.FileSize <= 0 {
		ctx.Reply(u, ext.ReplyTextString("Only Telegram documents with a known file size can be added to GDPlayer."), nil)
		return dispatcher.EndGroups
	}

	registered, err := control.Register(ctx, config.ValueOf.GDPlayerControlURL, config.ValueOf.GDPlayerSharedSecret, control.RegisterRequest{
		MessageID: messageIDUpdate.ID,
		FileName:  file.FileName,
		FileSize:  file.FileSize,
		MimeType:  file.MimeType,
	}, time.Duration(config.ValueOf.GDPlayerTimeoutSec)*time.Second)
	if err != nil {
		utils.Logger.Sugar().Errorf("GDPlayer asset registration failed: %v", err)
		ctx.Reply(u, ext.ReplyTextString("GDPlayer could not register this file. No Render streaming fallback is available."), nil)
		return dispatcher.EndGroups
	}

	text := styling.Code(registered.PlayerURL)
	markup := &tg.ReplyInlineMarkup{Rows: []tg.KeyboardButtonRow{{Buttons: []tg.KeyboardButtonClass{
		&tg.KeyboardButtonURL{Text: "Watch in GDPlayer", URL: registered.PlayerURL},
		&tg.KeyboardButtonURL{Text: "Download from GDPlayer", URL: registered.DownloadURL},
	}}}}
	_, err = ctx.Reply(u, ext.ReplyTextStyledText(text), &ext.ReplyOpts{
		Markup:           markup,
		NoWebpage:        false,
		ReplyToMessageId: u.EffectiveMessage.ID,
	})
	if err != nil {
		utils.Logger.Sugar().Error(err)
	}
	return dispatcher.EndGroups
}
