package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/VTGare/Eugen/database"
	"github.com/VTGare/Eugen/services"
	"github.com/VTGare/Eugen/utils"
	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
)

type StarboardEvent struct {
	guild       *database.Guild
	session     *discordgo.Session
	message     *discordgo.Message
	board       *database.Message
	channel     *discordgo.Channel
	addEvent    *discordgo.MessageReactionAdd
	removeEvent *discordgo.MessageReactionRemove
	deleteEvent *discordgo.MessageDelete
	selfstar    bool
}

func newStarboardEventAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) (*StarboardEvent, error) {
	guild := database.GuildCache[r.GuildID]
	message, err := s.ChannelMessage(r.ChannelID, r.MessageID)
	if err != nil {
		return nil, err
	}

	ch, err := s.Channel(r.ChannelID)
	if err != nil {
		return nil, err
	}

	return &StarboardEvent{guild: guild, message: message, channel: ch, session: s, addEvent: r, removeEvent: nil}, nil
}

func newStarboardEventRemove(s *discordgo.Session, r *discordgo.MessageReactionRemove) (*StarboardEvent, error) {
	guild := database.GuildCache[r.GuildID]
	message, err := s.ChannelMessage(r.ChannelID, r.MessageID)
	if err != nil {
		return nil, err
	}

	ch, err := s.Channel(r.ChannelID)
	if err != nil {
		return nil, err
	}

	return &StarboardEvent{guild: guild, message: message, channel: ch, session: s, addEvent: nil, removeEvent: r}, nil
}

func newStarboardEventDeleted(s *discordgo.Session, d *discordgo.MessageDelete) (*StarboardEvent, error) {
	guild := database.GuildCache[d.GuildID]

	ch, err := s.Channel(d.ChannelID)
	if err != nil {
		return nil, err
	}

	return &StarboardEvent{guild: guild, message: &discordgo.Message{ID: d.ID}, channel: ch, session: s, addEvent: nil, removeEvent: nil, deleteEvent: d}, nil
}

func (se *StarboardEvent) Run() error {
	var err error
	logrus.Infoln("Run(): invoked")

	se.board, err = database.Repost(se.channel.ID, se.message.ID)
	if err != nil {
		return err
	}
	logrus.Infof("Starboard: [%v %v]", se.board.Starboard.ChannelID, se.board.Starboard.MessageID)

	if se.deleteEvent != nil {
		se.deleteStarboard()
	} else if se.isStarboarded() {
		self, err := se.isSelfStar()
		if err != nil {
			return err
		}
		se.selfstar = self

		switch {
		case se.selfstar && !se.guild.Selfstar:
			return nil
		case se.addEvent != nil:
			se.incrementStarboard()
		case se.removeEvent != nil:
			se.decrementStarboard()
		}
	} else if se.addEvent != nil {
		self, err := se.isSelfStar()
		if err != nil {
			return err
		}
		se.selfstar = self

		se.createStarboard()
	}

	return nil
}

func (se *StarboardEvent) isStarboarded() bool {
	return se.board != nil
}

func (se *StarboardEvent) isSelfStar() (bool, error) {
	r := se.findReact()
	if r == nil {
		return false, nil
	}

	users, err := se.session.MessageReactions(se.message.ChannelID, se.message.ID, r.Emoji.APIName(), 100, "", "")
	if err != nil {
		return false, fmt.Errorf("MessageReactions(): %v", err)
	}

	for _, user := range users {
		if user.ID == se.message.Author.ID {
			return true, nil
		}
	}

	return false, nil
}

func (se *StarboardEvent) createStarboard() {
	required := se.guild.StarsRequired(se.addEvent.ChannelID)

	if react := se.findReact(); react != nil {
		if se.selfstar && !se.guild.Selfstar {
			react.Count--
		}

		if react.Count >= required {
			embed, resp, err := se.createEmbed(react)

			if err != nil {
				logrus.Warnln(err)
			}

			if embed != nil {
				logrus.Infof("Creating a new starboard. Guild: %v, channel: %v, message: %v", se.guild.Name, se.addEvent.ChannelID, se.addEvent.MessageID)
				starboard, err := se.session.ChannelMessageSendComplex(se.guild.StarboardChannel, embed)

				if resp != nil {
					resp.Body.Close()
				}

				handleError(se.session, se.addEvent.ChannelID, err)
				oPair := database.NewPair(se.message.ChannelID, se.message.ID)
				sPair := database.NewPair(starboard.ChannelID, starboard.ID)
				err = database.InsertOneMessage(database.NewMessage(&oPair, &sPair, se.addEvent.GuildID))
				handleError(se.session, se.addEvent.ChannelID, err)
			}
		}
	}
}

func (se *StarboardEvent) incrementStarboard() {
	if react := se.findReact(); react != nil {
		if se.selfstar && !se.guild.Selfstar {
			react.Count--
		}

		msg, err := se.session.ChannelMessage(se.board.Starboard.ChannelID, se.board.Starboard.MessageID)
		if err != nil {
			if strings.Contains(err.Error(), "404 Not Found") {
				logrus.Infoln("Unknown starboard cached. Removing.")
				err := database.DeleteMessage(&database.MessagePair{ChannelID: se.message.ChannelID, MessageID: se.message.ID})
				if err != nil {
					logrus.Warnln(err)
				}
				return
			}
			logrus.Warnln(err)
		} else {
			logrus.Infoln(fmt.Sprintf("Editing starboard (adding) %v in channel %v", msg.ID, msg.ChannelID))
			embed := se.editStarboard(msg, react)
			se.session.ChannelMessageEditEmbed(msg.ChannelID, msg.ID, embed)
		}
	}
}

func (se *StarboardEvent) decrementStarboard() {
	starboard, err := se.session.ChannelMessage(se.board.Starboard.ChannelID, se.board.Starboard.MessageID)
	if err != nil {
		if strings.Contains(err.Error(), "404 Not Found") {
			logrus.Infoln("Unknown starboard cached. Removing.")
			err := database.DeleteMessage(&database.MessagePair{ChannelID: se.message.ChannelID, MessageID: se.message.ID})
			if err != nil {
				logrus.Warnln(err)
			}
			return
		}
		logrus.Warnln(err)
	}

	if starboard == nil {
		logrus.Warnln("decrementStarboard(): nil starboard")
		return
	}

	required := se.guild.StarsRequired(se.removeEvent.ChannelID)
	if react := se.findReact(); react != nil {
		if se.selfstar && !se.guild.Selfstar {
			react.Count--
		}

		logrus.Infof("Editing starboard (subtracting) %v in channel %v", se.board.Starboard.MessageID, se.board.Starboard.ChannelID)
		if react.Count <= required/2 {
			err := se.session.ChannelMessageDelete(starboard.ChannelID, starboard.ID)
			if err != nil {
				logrus.Warnln(err)
			}
		} else {
			embed := se.editStarboard(starboard, react)
			_, err := se.session.ChannelMessageEditEmbed(starboard.ChannelID, starboard.ID, embed)
			if err != nil {
				logrus.Warnln(err)
			}
		}
	} else {
		err := se.session.ChannelMessageDelete(starboard.ChannelID, starboard.ID)
		if err != nil {
			logrus.Warnln(err)
		}
	}
}

func (se *StarboardEvent) deleteStarboard() error {
	var (
		original = true
	)

	if se.board == nil {
		original = false
		board, err := database.RepostByStarboard(se.channel.ID, se.message.ID)
		if err != nil {
			return err
		}
		if board != nil {
			se.board = board
		} else {
			return nil
		}
	}

	if ch, ok := starboardQueue[*se.board.Original]; ok {
		close(ch)
		delete(starboardQueue, *se.board.Original)
	}

	err := database.DeleteMessage(se.board.Original)
	if err != nil {
		logrus.Warnln(err)
	}

	logrus.Infof("Deleting starboard. ID: %v. Original: %v", se.deleteEvent.ID, original)
	if original {
		starboard, err := se.session.ChannelMessage(se.board.Starboard.ChannelID, se.board.Starboard.MessageID)
		if err != nil {
			return err
		}
		err = se.session.ChannelMessageDelete(starboard.ChannelID, starboard.ID)
		if err != nil {
			logrus.Warnln(err)
		}
	}
	return nil
}

func (se *StarboardEvent) createEmbed(react *discordgo.MessageReactions) (*discordgo.MessageSend, *http.Response, error) {
	var (
		resp *http.Response
	)

	t, _ := se.message.Timestamp.Parse()
	messageURL := fmt.Sprintf("https://discord.com/channels/%v/%v/%v", se.addEvent.GuildID, se.addEvent.ChannelID, se.message.ID)

	msg := &discordgo.MessageSend{}
	footer := &discordgo.MessageEmbedFooter{}
	if se.guild.IsGuildEmoji() {
		footer.IconURL = emojiURL(react.Emoji)
		footer.Text = fmt.Sprintf("%v", react.Count)
	} else {
		footer.Text = fmt.Sprintf("%v %v", "⭐", react.Count)
	}

	if se.selfstar && se.guild.Selfstar {
		footer.Text += " | self-starred"
	}

	embed := &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    fmt.Sprintf("%v in #%v", se.message.Author.String(), se.channel.Name),
			URL:     messageURL,
			IconURL: se.message.Author.AvatarURL(""),
		},
		Color:       int(se.guild.EmbedColour),
		Description: se.message.Content,
		Fields:      []*discordgo.MessageEmbedField{{Name: "Original message", Value: fmt.Sprintf("[Click here desu~](%v)", messageURL), Inline: true}},
		Timestamp:   t.Format(time.RFC3339),
		Footer:      footer,
	}

	if len(se.message.Attachments) != 0 {
		if utils.ImageURLRegex.MatchString(se.message.Attachments[0].URL) {
			embed.Image = &discordgo.MessageEmbedImage{
				URL: se.message.Attachments[0].URL,
			}
		} else {
			var err error
			resp, err = http.Get(se.message.Attachments[0].URL)
			if err != nil {
				return nil, nil, err
			}
			lastInd := strings.LastIndex(se.message.Attachments[0].URL, "/")
			msg.Files = []*discordgo.File{
				{
					Name:   se.message.Attachments[0].URL[lastInd:],
					Reader: resp.Body,
				},
			}
		}
	} else if str := utils.VideoURLRegex.FindString(se.message.Content); str != "" {
		var err error

		uri := str
		if strings.HasSuffix(uri, "gifv") {
			uri = strings.Replace(uri, "gifv", "mp4", 1)
		}

		resp, err = http.Get(uri)
		if err != nil {
			return nil, nil, err
		}
		lastInd := strings.LastIndex(str, "/")
		msg.Files = []*discordgo.File{
			{
				Name:   uri[lastInd:],
				Reader: resp.Body,
			},
		}
		embed.Description = strings.Replace(embed.Description, str, "", 1)
	} else if str := utils.ImageURLRegex.FindString(se.message.Content); str != "" {
		embed.Image = &discordgo.MessageEmbedImage{
			URL: str,
		}
		embed.Description = strings.Replace(embed.Description, str, "", 1)
	} else if tenor := findTenor(embed.Description); tenor != "" {
		res, err := services.Tenor(tenor)
		if err != nil {
			logrus.Warn(err)
		} else if len(res.Media) != 0 {
			embed.Description = strings.ReplaceAll(embed.Description, tenor, "")
			media := res.Media[0]
			embed.Image = &discordgo.MessageEmbedImage{
				URL: media.MediumGIF.URL,
			}
		}
	} else if len(se.message.Embeds) != 0 {
		emb := se.message.Embeds[0]
		if emb.Footer != nil {
			if strings.EqualFold(emb.Footer.Text, "twitter") {
				if twitter := utils.TwitterRegex.FindString(se.message.Content); twitter != "" {
					embed.Description = strings.Replace(embed.Description, twitter, "", 1)
					embed.Description += fmt.Sprintf("\n```\n%v\n```", emb.Description)
					embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Twitter", Value: fmt.Sprintf("[Click here desu~](%v)", twitter), Inline: true})
				}
				embed.Image = emb.Image
				if emb.Video != nil {
					embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Video", Value: fmt.Sprintf("[Click here desu~](%v)", emb.Video.URL), Inline: true})
				}
			}
		} else if emb.Provider != nil && strings.EqualFold(emb.Provider.Name, "youtube") {
			embed.Image = &discordgo.MessageEmbedImage{URL: emb.Thumbnail.URL}
			yt := utils.YoutubeRegex.FindString(embed.Description)
			embed.Description = strings.ReplaceAll(embed.Description, yt, "")
			embed.Description += "\n```" + emb.Title + "```"
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "YouTube", Value: fmt.Sprintf("[Click here desu~](%v)", emb.URL), Inline: true})
		} else if img := se.message.Embeds[0].Image; img != nil {
			if img.URL != "" {
				embed.Image = &discordgo.MessageEmbedImage{
					URL: se.message.Embeds[0].Image.URL,
				}
			}
		}
	}

	msg.Embed = embed
	return msg, resp, nil
}

func (se *StarboardEvent) findReact() *discordgo.MessageReactions {
	for _, react := range se.message.Reactions {
		if strings.ToLower(react.Emoji.APIName()) == strings.Trim(se.guild.StarEmote, "<:>") {
			return react
		}
	}
	return nil
}

func emojiURL(emoji *discordgo.Emoji) string {
	url := fmt.Sprintf("https://cdn.discordapp.com/emojis/%v.", emoji.ID)
	if emoji.Animated {
		url += "gif"
	} else {
		url += "png"
	}

	return url
}

func findTenor(content string) string {
	tenor := ""
	if ind := strings.Index(content, "https://tenor.com/view/"); ind != -1 {
		if ws := strings.IndexAny(content[ind:], " \n"); ws == -1 {
			tenor = content[ind:]
		} else {
			tenor = content[ind : ws+ind]
		}

		logrus.Info(tenor)
	}

	return tenor
}

func (se *StarboardEvent) editStarboard(msg *discordgo.Message, react *discordgo.MessageReactions) *discordgo.MessageEmbed {
	embed := msg.Embeds[0]

	if se.guild.IsGuildEmoji() {
		embed.Footer.Text = strconv.Itoa(react.Count)
	} else {
		embed.Footer.Text = fmt.Sprintf("⭐ %v", react.Count)
	}

	if se.selfstar && se.guild.Selfstar {
		embed.Footer.Text += " | self-starred"
	}

	return embed
}
