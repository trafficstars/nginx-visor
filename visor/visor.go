package visor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/flosch/pongo2"
	"github.com/trafficstars/registry"
)

var environment = map[string]string{
	"REGISTRY_DSN":     "http://127.0.0.1:8500?dc=dc1&refresh_interval=5",
	"TEMPLATES_DIR":    "/etc/nginx-visor/templates/",
	"NGINX_CONF_DIR":   "/etc/nginx/conf.d/",
	"NGINX_RELOAD_CMD": "service nginx reload",
	"NGINX_TEST_CMD":   "/usr/local/sbin/nginx -t",
}

func Run() error {
	for k, v := range environment {
		if env := os.Getenv(k); len(env) != 0 && env != v {
			environment[k] = env
		} else {
			log.Infof("Use default value [%s]: %s", k, v)
		}
	}
	log.Infof("Registry DSN: %s", environment["REGISTRY_DSN"])
	registry, err := registry.New(environment["REGISTRY_DSN"], []string{})
	if err != nil {
		return err
	}
	(&visor{
		hashes:    make(map[string]string),
		discovery: registry.Discovery(),
	}).run()
	return nil
}

type visor struct {
	hashes    map[string]string
	discovery registry.Discovery
}

func (v *visor) check() {
	items, err := v.discovery.Lookup(nil)
	if err != nil {
		log.Errorf("Lookup: %v", err)
		return
	}
	log.Debugf("Lookup: %d item(s)", len(items))
	services := make(map[string][]server)
	for _, item := range items {
		if item.Status == registry.SERVICE_STATUS_PASSING {
			log.Debugf("[%s] append: address=%s, port=%d, weight=%d", item.Name, item.Address, item.Port, serverWeight(&item))
			services[item.Name] = append(services[item.Name], server{
				Host:   item.Address,
				Port:   item.Port,
				Weight: serverWeight(&item),
			})
		} else {
			log.Debugf("[%s] backup: address=%s, port=%d", item.Name, item.Address, item.Port)
			services[item.Name] = append(services[item.Name], server{
				Host:   item.Address,
				Port:   item.Port,
				Backup: true,
			})
		}
	}
	var reload bool

	for service, servers := range services {
		if len(servers) == 0 {
			continue
		}
		switch {
		case len(servers) == 0:
			continue
		case len(servers) == 1:
			servers[0].Backup = false
		}
		hash := makeHash(servers)
		old, found := v.hashes[service]
		log.Debugf("Service [%s]: hash=%s, old=%s, found=%t", service, hash, old, found)
		if !found || hash != old {
			if err := v.makeConfig(service, servers); err == nil {
				v.hashes[service] = hash
				reload = true
			}
		} else {
			log.Infof("Service [%s] has not changed", service)
		}
	}
	if reload {
		v.reloadNginx()
	}
}

func (v *visor) makeConfig(service string, servers []server) error {
	if len(servers) == 0 {
		return nil
	}

	var (
		config   = filepath.Join(environment["NGINX_CONF_DIR"], service+".conf")
		template = filepath.Join(environment["TEMPLATES_DIR"], service+".tpl")
	)

	tpl, err := pongo2.FromFile(template)
	if err != nil {
		log.Warnf("Could not open template file [%s]: %v", template, err)
		return err
	}

	cnf, err := os.OpenFile(config, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		log.Errorf("Could not open configuration file [%s]: %v", config, err)
		return err
	}

	if err := tpl.ExecuteWriter(pongo2.Context{"servers": servers}, cnf); err != nil {
		log.Errorf("Could not write to configuration file [%s]: %v", config, err)
		return err
	}

	return nil
}

func (v *visor) reloadNginx() error {
	log.Info("Reload Nginx")
	{
		var (
			buffer bytes.Buffer
			fields = strings.Fields(environment["NGINX_TEST_CMD"])
			cmd    = exec.Command(fields[0], fields[1:]...)
		)
		cmd.Stderr = &buffer
		defer buffer.Reset()
		if err := cmd.Run(); err != nil {
			log.Errorf("Test cmd failed: %v\n%s", err, buffer.String())
			return err
		}
	}
	{
		var (
			buffer bytes.Buffer
			fields = strings.Fields(environment["NGINX_RELOAD_CMD"])
			cmd    = exec.Command(fields[0], fields[1:]...)
		)
		cmd.Stderr = &buffer
		defer buffer.Reset()
		if err := cmd.Run(); err != nil {
			log.Errorf("Reload cmd failed: %v\n%s", err, buffer.String())
			return err
		}
	}

	return nil
}

func (v *visor) run() {
	tick := time.Tick(5 * time.Second)
	for {
		select {
		case <-tick:
			v.check()
		}
	}
}

func serverWeight(s *registry.Service) int {
	weight := 1
	for _, tag := range s.Tags {
		if strings.HasPrefix(tag, "SERVICE_WEIGHT=") {
			if v, _ := strconv.ParseInt(strings.TrimPrefix(tag, "SERVICE_WEIGHT="), 10, 32); v != 0 {
				weight = int(v)
			}
		}
	}
	return weight
}

func makeHash(servers []server) string {
	fields := make([]string, 0, len(servers))
	for _, server := range servers {
		fields = append(fields, fmt.Sprintf("%s:%d?w=%d&b=%t", server.Host, server.Port, server.Weight, server.Backup))
	}
	sort.Strings(fields)
	return strings.Join(fields, ",")
}

type server struct {
	Host   string
	Port   int
	Weight int
	Backup bool
}
