package detection

var builtinContextValues = map[string][]string{
	"ctx:web-runtime-binaries":     {"nginx", "apache2", "httpd", "php-fpm", "gunicorn", "uwsgi", "tomcat", "node", "nodejs"},
	"ctx:shell-binaries":           {"sh", "bash", "dash", "zsh", "ksh", "ash"},
	"ctx:download-client-binaries": {"curl", "wget"},
	"ioc:c2-download-port-feed":    {"8080"},
}

func builtinContentValues(ref string) []string {
	return append([]string(nil), builtinContextValues[ref]...)
}
