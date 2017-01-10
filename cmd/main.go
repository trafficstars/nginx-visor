package main

import (
	"net/http"
	_ "net/http/pprof"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/trafficstars/nginx-visor/visor"
)

func main() {
	var formatter log.Formatter = &log.JSONFormatter{
		TimestampFormat: "2006-01-02 15:04:05 MST",
	}

	if log.IsTerminal() {
		formatter = &log.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05 MST",
		}
	}
	log.SetFormatter(formatter)

	if level, err := log.ParseLevel(os.Getenv("LOG_LEVEL")); err == nil {
		log.SetLevel(level)
		if level == log.DebugLevel {
			go func() {
				log.Debug("PPROF listen :6060")
				log.Debug(http.ListenAndServe(":6060", nil))
			}()
		}
	}

	if err := visor.Run(); err != nil {
		log.Fatalf("Could not runing nginx-visor: %v", err)
	}
}
