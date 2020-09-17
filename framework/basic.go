package framework

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/VTGare/Eugen/database"
	"github.com/VTGare/Eugen/utils"
	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func init() {
	basicGroup := CommandGroup{
		Name:        "basic",
		Description: "General purpose commands.",
		NSFW:        false,
		Commands:    make(map[string]Command),
		IsVisible:   true,
	}

	pingCommand := newCommand("ping", "Checks if Boe Tea is online and sends response time back.")
	pingCommand.setExec(ping)
	helpCommand := newCommand("help", "Sends this message. Use ``{prefix}help <group name> <command name>`` for more info about specific commands. ``{prefix}help <group>`` to list commands in a group.")
	helpCommand.setExec(help)
	setCommand := newCommand("set", "Show server's settings or change them.").setExec(set).setAliases("settings", "config", "cfg").setHelp(&HelpSettings{
		IsVisible: true,
		ExtendedHelp: []*discordgo.MessageEmbedField{
			{
				Name:  "Usage",
				Value: "{prefix}set ``<setting>`` ``<new setting>``",
			},
			{
				Name:  "prefix",
				Value: "Changes bot's prefix. Maximum ***5 characters***. If last character is a letter whitespace is assumed (takes one character).",
			},
			{
				Name:  "enabled",
				Value: "Starboard functionality switch, accepts ***f or false (case-insensitive)*** to disable and ***t or true*** to enable.",
			},
			{
				Name:  "starboard",
				Value: "Starboard channel. Required for starboard to work. Accepts channel ID or channel mention.",
			},
			{
				Name:  "emote",
				Value: "Starboard reaction emote.",
			},
			{
				Name:  "stars",
				Value: "Stars required to repost a message to starboard channel.",
			},
		},
	}).setGuildOnly(true)

	banCommand := newCommand("ban", "Bans a channel").setExec(ban).setGuildOnly(true)
	unbanCommand := newCommand("unban", "Unbans a channel").setExec(unban).setGuildOnly(true)
	reqCommand := newCommand("req", "Sets per channel star requirement").setExec(req).setGuildOnly(true).setAliases("requirement", "channelstars", "channelset")
	reqCommand.Help.ExtendedHelp = []*discordgo.MessageEmbedField{
		{
			Name:  "Usage",
			Value: "e!req <channel id or mention> <star requirement>",
		},
		{
			Name:  "Channel ID or mention",
			Value: "Required. It must be a channel on this server!",
		},
		{
			Name:  "Star requirement",
			Value: "Required. It must be an integer greater than or equal to 1 or ``default`` to remove a custom star requirement.",
		},
	}

	inviteCmd := newCommand("invite", "Sends an invite link").setExec(invite)
	setupCommand := newCommand("setup", "Starts an interactive Eugen setup process.").setExec(setup).setGuildOnly(true)

	basicGroup.addCommand(pingCommand)
	basicGroup.addCommand(helpCommand)
	basicGroup.addCommand(setCommand)
	basicGroup.addCommand(banCommand)
	basicGroup.addCommand(unbanCommand)
	basicGroup.addCommand(reqCommand)
	basicGroup.addCommand(inviteCmd)
	basicGroup.addCommand(setupCommand)
	CommandGroups["basic"] = basicGroup
}

func ping(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	_, err := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(":ping_pong: Pong! Latency: ***%v***", s.HeartbeatLatency().Round(1*time.Millisecond)))
	if err != nil {
		return err
	}
	return nil
}

func help(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	guild := database.GuildCache[m.GuildID]

	embed := &discordgo.MessageEmbed{
		Description: fmt.Sprintf("Use ``%vhelp <command name>`` for extended help on specific commands.", guild.Prefix),
		Color:       utils.EmbedColor,
		Timestamp:   utils.EmbedTimestamp(),
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: "https://i.imgur.com/OZ1Al5h.png",
		},
	}

	switch len(args) {
	case 0:
		embed.Title = "Help"
		for _, group := range CommandGroups {
			if group.IsVisible {
				unique := make(map[string]bool)
				for _, command := range group.Commands {
					if _, ok := unique[command.Name]; !ok {
						field := &discordgo.MessageEmbedField{
							Name:  command.Name,
							Value: command.createHelp(guild.Prefix),
						}

						unique[command.Name] = true
						embed.Fields = append(embed.Fields, field)
					}
				}
			}
		}
	case 1:
		found := false
		for _, group := range CommandGroups {
			if command, ok := group.Commands[args[0]]; ok {
				if len(command.Help.ExtendedHelp) > 0 && command.Help.IsVisible {
					found = true
					embed.Title = fmt.Sprintf("%v command extended help", command.Name)
					embed.Fields = command.createExtendedHelp(guild.Prefix)
				}
			}
		}

		if !found {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Command %v either doesn't have extended help info or doesn't exist.", args[0]))
			return nil
		}
	default:
		return errors.New("incorrect command usage. Example: bt!help <command name>")
	}

	s.ChannelMessageSendEmbed(m.ChannelID, embed)
	return nil
}

func ban(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	if len(args) == 0 {
		return utils.ErrNotEnoughArguments
	}

	guild := database.GuildCache[m.GuildID]
	for _, arg := range args {
		if strings.HasPrefix(arg, "<#") {
			arg = strings.Trim(arg, "<#>")
		}
		ch, err := s.Channel(arg)
		if err != nil {
			return err
		}
		if ch.GuildID == m.GuildID {
			if !guild.IsBanned(ch.ID) {
				err := database.BanChannel(ch.GuildID, ch.ID)
				if err != nil {
					return err
				}
			}
		}
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Successfully banned following channels: %v", args))
	return nil
}

func unban(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	if len(args) == 0 {
		return utils.ErrNotEnoughArguments
	}

	guild := database.GuildCache[m.GuildID]
	for _, arg := range args {
		if strings.HasPrefix(arg, "<#") {
			arg = strings.Trim(arg, "<#>")
		}
		ch, err := s.Channel(arg)
		if err != nil {
			return err
		}
		if ch.GuildID == m.GuildID {
			if guild.IsBanned(ch.ID) {
				err := database.UnbanChannel(ch.GuildID, ch.ID)
				if err != nil {
					return err
				}
			}
		}
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Successfully unbanned following channels: %v", args))
	return nil
}

func req(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	if len(args) < 2 {
		return utils.ErrNotEnoughArguments
	}
	channelID := ""

	channels, err := s.GuildChannels(m.GuildID)
	if err != nil {
		return err
	}

	if strings.HasPrefix(args[0], "<#") {
		channelID = strings.Trim(args[0], "<#>")
	}

	if !utils.IsValidChannel(channelID, channels) {
		return errors.New("you're trying to modify a foreign channel")
	}

	if args[1] == "default" {
		g := database.GuildCache[m.GuildID]

		f := false
		for _, ch := range g.ChannelSettings {
			if ch.ID == channelID {
				f = true
			}
		}

		if f {
			err = database.UnsetStarRequirement(m.GuildID, channelID)
			if err != nil {
				return err
			}
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Successfully reset <#%v> settings to defaults", channelID))
		} else {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Can't reset <#%v> to defaults, channel doesn't have star requirements set.", channelID))
		}
	} else {
		stars, err := strconv.Atoi(args[1])
		if err != nil {
			return utils.ErrParsingArgument
		}

		if stars < 1 {
			return fmt.Errorf("Star requirement should be >= 1, provided star requirement is %v", stars)
		}

		err = database.SetStarRequirement(m.GuildID, channelID, stars)
		if err != nil {
			return fmt.Errorf("database error\n%v", err)
		}
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Successfully set <#%v> star requirement to %v", channelID, stars))
	}
	return nil
}

func set(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	switch len(args) {
	case 0:
		showGuildSettings(s, m)
	case 2:
		isAdmin, err := utils.MemberHasPermission(s, m.GuildID, m.Author.ID, discordgo.PermissionAdministrator)
		if err != nil {
			return err
		}
		if !isAdmin {
			return utils.ErrNoPermission
		}

		setting := args[0]
		newSetting := strings.ToLower(args[1])

		var passedSetting interface{}
		switch setting {
		case "enabled":
			passedSetting, err = strconv.ParseBool(newSetting)
		case "selfstar":
			passedSetting, err = strconv.ParseBool(newSetting)
		case "color":
			if passedSetting, err = strconv.ParseInt(newSetting, 0, 32); err != nil {
				if passedSetting, err = strconv.ParseInt("0x"+newSetting, 0, 32); err != nil {
					return fmt.Errorf("unable to parse %v to a number", newSetting)
				}
			}
			if passedSetting.(int64) > 16777215 || passedSetting.(int64) < 0 {
				return errors.New("non-existing decimal color, it should be in range from 0 to 16777215")
			}
		case "prefix":
			if unicode.IsLetter(rune(newSetting[len(newSetting)-1])) {
				passedSetting = newSetting + " "
			} else {
				passedSetting = newSetting
			}

			if len(passedSetting.(string)) > 5 {
				return errors.New("new prefix is too long")
			}
		case "stars":
			passedSetting, err = strconv.Atoi(newSetting)
		case "emote":
			emoji, err := utils.GetEmoji(s, m.GuildID, newSetting)
			if err != nil {
				return errors.New("argument's either global emoji or not one at all")
			}
			passedSetting = emoji
		case "starboard":
			if strings.HasPrefix(newSetting, "<#") {
				newSetting = strings.TrimSuffix(strings.TrimPrefix(newSetting, "<#"), ">")
			}
			ch, err := s.Channel(newSetting)
			if err != nil {
				return err
			}
			if ch.GuildID != m.GuildID {
				return errors.New("can't assign starboard to a channel from a foreign server")
			}

			passedSetting = newSetting
		default:
			return errors.New("unknown setting " + setting)
		}

		if err != nil {
			return err
		}

		err = changeSetting(m.GuildID, setting, passedSetting)
		if err != nil {
			return err
		}
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Successfully changed ``%v`` to ``%v``", setting, newSetting))
	default:
		return errors.New("incorrect command usage. Please use e!help set command for more information")
	}

	return nil
}

func showGuildSettings(s *discordgo.Session, m *discordgo.MessageCreate) {
	settings := database.GuildCache[m.GuildID]
	guild, _ := s.Guild(settings.ID)

	banned := strings.Join(utils.Map(settings.BannedChannels, func(s string) string {
		return fmt.Sprintf("<#%v>", s)
	}), " | ")
	if banned == "" {
		banned = "none"
	}

	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Title:       "Current settings",
		Description: guild.Name,
		Color:       int(settings.EmbedColour),
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  "Starboard",
				Value: utils.FormatBool(settings.Enabled),
			},
			{
				Name:  "Settings",
				Value: fmt.Sprintf("Starboard: <#%v> | Emote: %v | Min stars: %v | Prefix: %v | Color: %v | Selfstar: %v", settings.StarboardChannel, settings.StarEmote, settings.MinimumStars, settings.Prefix, settings.EmbedColour, utils.FormatBool(settings.Selfstar)),
			},
			{
				Name:  "Unique star requirements",
				Value: settings.ChannelSettingsToString(),
			},
			{
				Name:  "Banned channels",
				Value: banned,
			},
		},
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: guild.IconURL(),
		},
		Timestamp: utils.EmbedTimestamp(),
	})
}

func changeSetting(guildID, setting string, newSetting interface{}) error {
	col := database.DB.Collection("guilds")

	res := col.FindOneAndUpdate(context.Background(), bson.M{
		"guild_id": guildID,
	}, bson.M{
		"$set": bson.M{
			setting:      newSetting,
			"updated_at": time.Now(),
		},
	}, options.FindOneAndUpdate().SetReturnDocument(options.After))

	guild := &database.Guild{}
	err := res.Decode(guild)
	if err != nil {
		return err
	}

	database.GuildCache[guildID] = guild
	return nil
}

func invite(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	embed := &discordgo.MessageEmbed{
		Title:       "Thanks for spreading the word!",
		Description: "Eugen loves you 💖\nhttps://discord.com/api/oauth2/authorize?client_id=738399095378673786&permissions=379968&scope=bot",
		Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: s.State.User.AvatarURL("")},
		Color:       utils.EmbedColor,
		Timestamp:   utils.EmbedTimestamp(),
	}

	s.ChannelMessageSendEmbed(m.ChannelID, embed)
	return nil
}

func setup(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
	ok, err := utils.MemberHasPermission(s, m.GuildID, m.Author.ID, 8)
	if err != nil {
		return err
	}

	if !ok {
		return fmt.Errorf("You don't have permission to run this command.")
	}

	var (
		guild     = database.GuildCache[m.GuildID]
		step      = 0
		done      bool
		exit      bool
		starboard string
		selfstar  bool
		minstars  int
		emote     string
		colour    int64
	)

	verifyChannel := func(chID string) bool {
		if !strings.HasPrefix(chID, "<#") || !strings.HasSuffix(chID, ">") {
			return false
		}

		chID = strings.Trim(chID, "<#>")
		ch, err := s.Channel(chID)
		if err != nil {
			logrus.Warnln(err)
			return false
		}

		if ch.GuildID != m.GuildID {
			return false
		}

		return true
	}

	verifyColour := func(colour string) (int64, bool) {
		var c int64
		var err error

		if c, err = strconv.ParseInt(colour, 0, 32); err != nil {
			if c, err = strconv.ParseInt("0x"+colour, 0, 32); err != nil {
				return 0, false
			}
		}
		if c > 16777215 || c < 0 {
			return 0, false
		}

		return c, true
	}

	steps := []func() (bool, error){
		func() (bool, error) {
			embed := utils.BaseEmbed(s)
			embed.Title = "Eugen Setup | Step 1: Starboard channel"

			var sb strings.Builder
			sb.WriteString("To complete this step **mention a starboard channel.**\n\nType ``cancel`` or ``exit`` to quit the setup.")
			embed.Description = sb.String()

			res := ""
			flag := false
			for !(flag || res == "cancel" || res == "exit") {
				res = utils.CreatePrompt(s, m, embed)
				flag = verifyChannel(res)
			}

			if res == "cancel" || res == "exit" {
				return false, nil
			}

			starboard = res
			step++

			return true, nil
		},
		func() (bool, error) {
			embed := utils.BaseEmbed(s)
			embed.Title = "Eugen Setup | Step 2: Minimum stars"
			var sb strings.Builder
			sb.WriteString("**Current settings:**\n")
			sb.WriteString(fmt.Sprintf("Starboard channel: %v\n", starboard))
			sb.WriteString("\nTo complete this step **type an integer number**.\n")
			sb.WriteString("\nType ``cancel`` or ``exit`` to cancel the setup.\nType ``previous`` to come back to a previous step")
			embed.Description = sb.String()

			res := ""
			flag := false
			for !(flag || res == "cancel" || res == "exit" || res == "previous") {
				res = utils.CreatePrompt(s, m, embed)
				num, err := strconv.Atoi(res)
				if err == nil {
					flag = true
					minstars = num
				}
			}

			if res == "cancel" || res == "exit" {
				return false, nil
			}

			if res == "previous" {
				step--
				return true, nil
			}

			step++
			return true, nil
		},
		func() (bool, error) {
			embed := utils.BaseEmbed(s)
			embed.Title = "Eugen Setup | Step 3: Star emote"
			var sb strings.Builder
			sb.WriteString("**Current settings:**\n")
			sb.WriteString(fmt.Sprintf("Starboard channel: %v\n", starboard))
			sb.WriteString(fmt.Sprintf("Minimum stars: %v\n", minstars))
			sb.WriteString("\nTo complete this step **send a guild emote or type default.**\n")
			sb.WriteString("\nType ``cancel`` or ``exit`` to cancel the setup.\nType ``previous`` to come back to a previous step")
			embed.Description = sb.String()

			res := ""
			flag := false
			for !(flag || res == "cancel" || res == "exit" || res == "previous" || res == "default") {
				res = utils.CreatePrompt(s, m, embed)
				e, err := utils.GetEmoji(s, m.GuildID, res)
				if err == nil {
					flag = true
					emote = e
				}
			}

			if res == "cancel" || res == "exit" {
				return false, nil
			}

			if res == "default" {
				emote = "⭐"
			}

			if res == "previous" {
				step--
				return true, nil
			}

			step++
			return true, nil
		},
		func() (bool, error) {
			embed := utils.BaseEmbed(s)
			embed.Title = "Eugen Setup | Step 4: Selfstar"
			var sb strings.Builder
			sb.WriteString("**Current settings:**\n")
			sb.WriteString(fmt.Sprintf("Starboard channel: %v\n", starboard))
			sb.WriteString(fmt.Sprintf("Minimum stars: %v\n", minstars))
			sb.WriteString(fmt.Sprintf("Emote: %v\n", emote))
			sb.WriteString("\nTo complete this step **type ``true`` to allow self-starring or ``false`` to not count self-stars.**\n")
			sb.WriteString("\nType ``cancel`` or ``exit`` to cancel the setup.\nType ``previous`` to come back to a previous step")
			embed.Description = sb.String()

			res := ""
			for !(res == "true" || res == "false" || res == "cancel" || res == "exit" || res == "previous") {
				res = utils.CreatePrompt(s, m, embed)
			}

			if res == "cancel" || res == "exit" {
				return false, nil
			}

			if res == "true" {
				selfstar = true
			} else if res == "false" {
				selfstar = false
			}

			if res == "previous" {
				step--
				return true, nil
			}

			step++
			return true, nil
		},
		func() (bool, error) {
			embed := utils.BaseEmbed(s)
			embed.Title = "Eugen Setup | Step 5: Embed colour"
			var sb strings.Builder
			sb.WriteString("**Current settings:**\n")
			sb.WriteString(fmt.Sprintf("Starboard channel: %v\n", starboard))
			sb.WriteString(fmt.Sprintf("Minimum stars: %v\n", minstars))
			sb.WriteString(fmt.Sprintf("Emote: %v\n", emote))
			sb.WriteString(fmt.Sprintf("Self-star: %v\n", utils.FormatBool(selfstar)))
			sb.WriteString("\nTo complete this step **send a hexadecimal colour or integer up to 16777215 or default.**\n")
			sb.WriteString("\nType ``cancel`` or ``exit`` to cancel the setup.\nType ``previous`` to come back to a previous step")
			embed.Description = sb.String()

			res := ""
			flag := false
			for !(flag || res == "true" || res == "false" || res == "cancel" || res == "exit" || res == "previous" || res == "default") {
				res = utils.CreatePrompt(s, m, embed)
				colour, flag = verifyColour(res)
			}

			if res == "cancel" || res == "exit" {
				return false, nil
			}

			if res == "default" {
				colour = 16744576
			}

			if res == "previous" {
				step--
				return true, nil
			}

			done = true
			return true, nil
		},
	}

	for !done {
		success, err := steps[step]()
		if err != nil {
			return err
		}
		if !success {
			exit = true
			done = true
		}
	}

	guild.Enabled = true
	guild.StarboardChannel = starboard
	guild.MinimumStars = minstars
	guild.StarEmote = emote
	guild.Selfstar = selfstar
	guild.EmbedColour = colour
	guild.UpdatedAt = time.Now()

	err = database.ReplaceGuild(guild)
	if err != nil {
		logrus.Warnf("ReplaceGuild(): %v", err)
	}

	embed := utils.BaseEmbed(s)
	if !exit && err == nil {
		embed.Title = "✅ Successfully setup Eugen!"
		embed.Fields = []*discordgo.MessageEmbedField{
			{Name: "Starboard channel", Value: starboard},
			{Name: "Minimum stars", Value: fmt.Sprintf("%v", minstars)},
			{Name: "Emote", Value: emote},
			{Name: "Self-star", Value: utils.FormatBool(selfstar)},
			{Name: "Embed colour", Value: "applied to this embed :)"}}
		embed.Color = int(colour)
	} else {
		reason := ""
		if exit {
			reason = "User manually cancelled setup."
		} else {
			reason = "Error occured while setting up. Please contact bot creator at VTGare#3370"
		}
		embed.Title = "❎ Failed to setup Eugen."
		embed.Fields = []*discordgo.MessageEmbedField{{"Reason", reason, false}}
	}

	s.ChannelMessageSendEmbed(m.ChannelID, embed)
	return nil
}
