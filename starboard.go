package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/VTGare/Eugen/database"
	"github.com/VTGare/Eugen/services"
	"github.com/VTGare/Eugen/utils"
	"github.com/VTGare/embeds"
	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
	"mvdan.cc/xurls/v2"
)

type StarboardEvent struct {
	React       *discordgo.MessageReactions
	guild       *database.Guild
	session     *discordgo.Session
	message     *discordgo.Message
	board       *database.Message
	addEvent    *discordgo.MessageReactionAdd
	removeEvent *discordgo.MessageReactionRemove
	deleteEvent *discordgo.MessageDelete
	selfstar    bool
}

type StarboardFile struct {
	Name      string
	URL       string
	Thumbnail *os.File
	Resp      *http.Response
}

func newStarboardEventAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd, msg *discordgo.Message, emote *discordgo.MessageReactions) (*StarboardEvent, error) {
	guild := database.GuildCache[r.GuildID]
	se := &StarboardEvent{guild: guild, message: msg, session: s, addEvent: r, removeEvent: nil, React: emote}

	return se, nil
}

func newStarboardEventRemove(s *discordgo.Session, r *discordgo.MessageReactionRemove, msg *discordgo.Message) (*StarboardEvent, error) {
	guild := database.GuildCache[r.GuildID]

	emote := FindReact(msg, guild.StarEmote)
	se := &StarboardEvent{guild: guild, message: msg, session: s, addEvent: nil, removeEvent: r, React: emote}

	return se, nil
}

func newStarboardEventDeleted(s *discordgo.Session, d *discordgo.MessageDelete) (*StarboardEvent, error) {
	guild := database.GuildCache[d.GuildID]

	return &StarboardEvent{guild: guild, message: &discordgo.Message{ID: d.ID, ChannelID: d.ChannelID}, session: s, addEvent: nil, removeEvent: nil, deleteEvent: d}, nil
}

func (se *StarboardEvent) Run() error {
	var err error

	se.board, err = database.Repost(se.message.ChannelID, se.message.ID)
	if err != nil {
		return err
	}

	if se.deleteEvent != nil {
		se.deleteStarboard()
	} else if se.isStarboarded() {
		self, err := se.isSelfStar()
		if err != nil {
			return err
		}
		se.selfstar = self

		switch {
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
	if se.React == nil {
		return false, nil
	}

	users, err := se.session.MessageReactions(se.message.ChannelID, se.message.ID, se.React.Emoji.APIName(), 100, "", "")
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

func (se *StarboardEvent) createStarboard() error {
	required := se.guild.StarsRequired(se.addEvent.ChannelID)
	if react := se.React; react != nil {
		if se.selfstar && !se.guild.Selfstar {
			react.Count--
		}

		if react.Count >= required {
			ch, err := se.session.Channel(se.message.ChannelID)
			if err != nil {
				return err
			}

			embed, resp, err := se.createEmbed(react, ch)
			if err != nil {
				return err
			}

			if embed != nil {
				logrus.Infof("Creating a new starboard. Guild: %v, channel: %v, message: %v", se.guild.Name, se.addEvent.ChannelID, se.addEvent.MessageID)

				starboardChannel := ""
				if ch.NSFW && se.guild.NSFWStarboardChannel != "" {
					starboardChannel = se.guild.NSFWStarboardChannel
				} else {
					starboardChannel = se.guild.StarboardChannel
				}

				starboard, err := se.session.ChannelMessageSendComplex(starboardChannel, embed)
				if err != nil {
					return err
				}

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

	return nil
}

func (se *StarboardEvent) incrementStarboard() {
	if react := se.React; react != nil {
		if se.selfstar && !se.guild.Selfstar {
			react.Count--
		}

		msg, err := se.session.ChannelMessage(se.board.Starboard.ChannelID, se.board.Starboard.MessageID)
		if err != nil {
			if strings.Contains(err.Error(), "404 Not Found") {
				logrus.Infoln("Unknown starboard cached. Removing.")
				err := database.DeleteMessage(&database.MessagePair{ChannelID: se.message.ChannelID, MessageID: se.message.ID})
				if err != nil {
					logrus.Warnln("database.DeleteMessage(): ", err)
				}
				return
			}
			logrus.Warnln("se.session.ChannelMessage(): ", err)
		} else {
			embed := se.editStarboard(msg, react)
			if embed != nil {
				logrus.Infoln(fmt.Sprintf("Editing starboard (adding) %v in channel %v", msg.ID, msg.ChannelID))
				se.session.ChannelMessageEditEmbed(msg.ChannelID, msg.ID, embed)
			}
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
				logrus.Warnln("database.DeleteMessage(): ", err)
			}
			return
		}
		logrus.Warnln("se.session.ChannelMessage(): ", err)
	}

	if starboard == nil {
		logrus.Warnln("decrementStarboard(): nil starboard")
		return
	}

	required := se.guild.StarsRequired(se.removeEvent.ChannelID)
	if react := se.React; react != nil {
		if se.selfstar && !se.guild.Selfstar {
			react.Count--
		}

		if react.Count <= required/2 {
			err := se.session.ChannelMessageDelete(starboard.ChannelID, starboard.ID)
			if err != nil {
				logrus.Warnln("se.session.ChannelMessageDelete():", err)
			}
		} else {
			embed := se.editStarboard(starboard, react)
			if embed != nil {
				logrus.Infof("Editing starboard (subtracting) %v in channel %v", se.board.Starboard.MessageID, se.board.Starboard.ChannelID)
				_, err := se.session.ChannelMessageEditEmbed(starboard.ChannelID, starboard.ID, embed)
				if err != nil {
					logrus.Warnln("se.session.ChannelMessageEditEmbed():", err)
				}
			}
		}
	} else {
		err := se.session.ChannelMessageDelete(starboard.ChannelID, starboard.ID)
		if err != nil {
			logrus.Warnln("se.session.ChannelMessageDelete(): ", err)
		}
	}
}

func (se *StarboardEvent) deleteStarboard() error {
	var (
		original = true
	)

	if se.board == nil {
		original = false
		board, err := database.RepostByStarboard(se.deleteEvent.ChannelID, se.message.ID)
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
		logrus.Warnln("database.DeleteMessage():", err)
	}

	logrus.Infof("Deleting starboard. ID: %v. Original: %v", se.deleteEvent.ID, original)
	if original {
		starboard, err := se.session.ChannelMessage(se.board.Starboard.ChannelID, se.board.Starboard.MessageID)
		if err != nil {
			return err
		}
		err = se.session.ChannelMessageDelete(starboard.ChannelID, starboard.ID)
		if err != nil {
			logrus.Warnln("se.session.ChannelMessageDelete():", err)
		}
	}
	return nil
}

func (se *StarboardEvent) createEmbed(react *discordgo.MessageReactions, ch *discordgo.Channel) (*discordgo.MessageSend, *http.Response, error) {
	var (
		eb         = embeds.NewBuilder()
		resp       *http.Response
		t, _       = se.message.Timestamp.Parse()
		messageURL = fmt.Sprintf("https://discord.com/channels/%v/%v/%v", se.addEvent.GuildID, se.addEvent.ChannelID, se.message.ID)
		msg        = &discordgo.MessageSend{}
		content    = se.message.Content
		rx         = xurls.Strict()
		URLs       = make([]*EugenURL, 0)
	)

	for _, uri := range rx.FindAllString(content, -1) {
		parsed, err := url.Parse(uri)
		if err != nil {
			continue
		}

		eu := &EugenURL{
			URL: parsed,
		}

		switch {
		case strings.HasSuffix(parsed.Path, "jpg") || strings.HasSuffix(parsed.Path, "png") || strings.HasSuffix(parsed.Path, "jpeg") || strings.HasSuffix(parsed.Path, "webp"):
			eu.Type = ImageURL
		case strings.HasSuffix(parsed.Path, "mp4") || strings.HasSuffix(parsed.Path, "webm") || strings.HasSuffix(parsed.Path, "mp4") || strings.HasSuffix(parsed.Path, "mov") || strings.HasSuffix(parsed.Path, "gifv"):
			eu.Type = VideoURL
		case strings.Contains(parsed.Host, "imgur"):
			eu.Type = ImgurURL
		case strings.Contains(parsed.String(), "tenor.com/view"):
			eu.Type = TenorURL
		default:
			continue
		}

		URLs = append(URLs, eu)
	}

	eb.Author(fmt.Sprintf("%v in #%v", se.message.Author.String(), ch.Name), messageURL, se.message.Author.AvatarURL(""))
	eb.Color(int(se.guild.EmbedColour))
	eb.Timestamp(t)
	eb.AddField("Original message", fmt.Sprintf("[Click here desu~](%v)", messageURL), true)
	if se.guild.IsGuildEmoji() {
		text := fmt.Sprintf("%v", react.Count)
		if se.selfstar && se.guild.Selfstar {
			text += " | self-starred"
		}

		eb.Footer(text, emojiURL(react.Emoji))
	} else {
		text := fmt.Sprintf("%v %v", "⭐", react.Count)
		if se.selfstar && se.guild.Selfstar {
			text += " | self-starred"
		}

		eb.Footer(text, "")
	}

	if ref := se.message.MessageReference; ref != nil {
		uri := fmt.Sprintf("https://discord.com/channels/%v/%v/%v", ref.GuildID, ref.ChannelID, ref.MessageID)
		eb.AddField("Reply to", fmt.Sprintf("[Click here desu~](%v)", uri), true)
	}

	switch {
	case len(se.message.Attachments) != 0:
		var (
			first = se.message.Attachments[0]
			rest  = se.message.Attachments[1:]
		)

		if utils.ImageURLRegex.MatchString(first.URL) {
			eb.Image(first.URL)
		} else {
			file, err := se.downloadFile(first.URL)
			if err != nil {
				return nil, nil, err
			}

			if file.Resp != nil {
				resp = file.Resp
				msg.Files = []*discordgo.File{
					{
						Name:   file.Name,
						Reader: file.Resp.Body,
					},
				}
			} else {
				eb.AddField("Attachment", fmt.Sprintf("[Click here desu~](%v)", first.URL), true)
			}
		}

		for ind, a := range rest {
			eb.AddField(fmt.Sprintf("Attachment %v", ind+2), fmt.Sprintf("[Click here desu~](%v)", a.URL), true)
		}
	case len(URLs) != 0:
		switch URLs[0].Type {
		case ImageURL:
			str := URLs[0].URL.String()
			eb.Image(str)
			content = strings.Replace(content, str, "", 1)
		case VideoURL:
			uri := URLs[0].URL.String()
			if strings.HasSuffix(uri, "gifv") {
				uri = strings.Replace(uri, "gifv", "mp4", 1)
			}

			video, err := se.downloadFile(uri)
			if err != nil {
				return nil, nil, err
			}

			if video.Resp != nil {
				resp = video.Resp
				msg.Files = []*discordgo.File{
					{
						Name:   video.Name,
						Reader: video.Resp.Body,
					},
				}
			} else {
				eb.AddField("Attachment", fmt.Sprintf("[Click here desu~](%v)", uri), true)
			}
			content = strings.Replace(content, uri, "", 1)
		case TenorURL:
			tenor := URLs[0].URL.String()
			res, err := services.Tenor(tenor)
			if err != nil {
				logrus.Warn(err)
			} else if len(res.Media) != 0 {
				content = strings.ReplaceAll(content, tenor, "")
				eb.Image(res.Media[0].MediumGIF.URL)
			}
		case ImgurURL:
			if len(se.message.Embeds) != 0 {
				emb := se.message.Embeds[0]
				if emb.Video != nil {
					file, err := se.downloadFile(emb.Video.URL)
					if err != nil {
						logrus.Warnln("se.dowloadFile():", err)
					}

					if file.Resp != nil {
						resp = file.Resp
						msg.Files = []*discordgo.File{
							{
								Name:   file.Name,
								Reader: file.Resp.Body,
							},
						}
					} else if emb.Thumbnail != nil {
						eb.Image(emb.Thumbnail.ProxyURL)
					}
				} else if emb.Thumbnail != nil {
					eb.Image(emb.Thumbnail.ProxyURL)
				}
			} else {
				eb.Image(fmt.Sprintf("https://i.imgur.com/%v.png", URLs[0].URL.Path))
			}

			content = strings.Replace(content, URLs[0].URL.String(), "", 1)
		}
	case len(se.message.Embeds) != 0:
		emb := se.message.Embeds[0]
		if emb.Footer != nil && strings.EqualFold(emb.Footer.Text, "twitter") {
			if twitter := utils.TwitterRegex.FindString(se.message.Content); twitter != "" {
				content = strings.Replace(content, twitter, "", 1)
				content += fmt.Sprintf("\n[%v](%v)\n```\n%v\n```", emb.Author.Name, emb.Author.URL, emb.Description)
				eb.AddField("Twitter", fmt.Sprintf("[Click here desu~](%v)", twitter), true)
			}

			if emb.Image != nil {
				eb.Image(emb.Image.URL)
			}

			if emb.Video != nil {
				eb.AddField("Twitter video", fmt.Sprintf("[Click here desu~](%v)", emb.Video.URL), true)
			}
		} else if emb.Provider != nil && strings.EqualFold(emb.Provider.Name, "youtube") {
			eb.Image(emb.Thumbnail.URL)
			yt := utils.YoutubeRegex.FindString(content)
			content = strings.Replace(content, yt, "", 1)

			content += "\n```" + emb.Title + "```"
			eb.AddField("YouTube", fmt.Sprintf("[Click here desu~](%v)", emb.URL), true)
		} else {
			if emb.Description != "" {
				content += "\n"
				if emb.Title != "" {
					content += fmt.Sprintf("**%v**\n", emb.Title)
				}
				content += fmt.Sprintf("%v", emb.Description)
			}

			if emb.Image != nil && emb.Image.URL != "" {
				eb.Image(emb.Image.URL)
			}
		}
	}

	eb.Description(content)
	msg.Embed = eb.Finalize()
	return msg, resp, nil
}

func FindReact(message *discordgo.Message, emote string) *discordgo.MessageReactions {
	for _, react := range message.Reactions {
		if strings.ToLower(react.Emoji.APIName()) == strings.Trim(emote, "<:>") {
			return react
		}
	}
	return nil
}

func (se *StarboardEvent) editStarboard(msg *discordgo.Message, react *discordgo.MessageReactions) *discordgo.MessageEmbed {
	embed := msg.Embeds[0]

	current, _ := strconv.Atoi(strings.Trim(embed.Footer.Text, "⭐ "))
	if current == react.Count {
		return nil
	}

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

func (se *StarboardEvent) downloadFile(uri string) (*StarboardFile, error) {
	var (
		file  = &StarboardFile{"", "", nil, nil}
		limit = int64(8388608)
	)

	head, err := http.Head(uri)
	if err != nil {
		return nil, err
	}

	g, err := se.session.Guild(se.addEvent.GuildID)
	if err == nil {
		if g.PremiumTier == discordgo.PremiumTier2 || g.PremiumTier == discordgo.PremiumTier3 {
			limit = int64(52428800)
		}
	} else {
		logrus.Warnf("downloadFile(): %v", err)
	}

	//if Content-Length is larger than 8MB | 50MB depending on boost level
	if head.ContentLength >= limit {
		file.URL = uri
		return file, nil
	}

	resp, err := http.Get(uri)
	if err != nil {
		return nil, err
	}
	begin := strings.LastIndex(uri, "/")
	end := strings.LastIndex(uri, "?")
	if end != -1 && end > begin {
		file.Name = uri[begin:end]
	} else {
		file.Name = uri[begin:]
	}

	file.Resp = resp

	return file, nil
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
