package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/VTGare/Eugen/database"
	"github.com/bwmarrin/discordgo"
	log "github.com/sirupsen/logrus"
)

var (
	dg *discordgo.Session
)

func init() {
	log.SetFormatter(&log.TextFormatter{})
}

func main() {
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatalln("BOT_TOKEN env variable doesn't exit")
	}

	var err error
	dg, err = discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalln("Error creating a session: ", err)
	}

	dg.AddHandler(onReady)
	dg.AddHandler(messageCreated)
	dg.AddHandler(guildCreated)
	dg.AddHandler(reactCreated)
	dg.AddHandler(guildDeleted)

	if err := dg.Open(); err != nil {
		log.Fatalln("Error opening connection,", err)
	}
	defer dg.Close()

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, syscall.SIGSEGV, syscall.SIGHUP)
	<-sc

	database.Client.Disconnect(context.Background())
}
