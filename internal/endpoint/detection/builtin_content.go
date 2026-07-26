package detection

var builtinContextValues = map[string][]string{
	"ctx:web-runtime-binaries":     {"nginx", "apache2", "httpd", "php-fpm", "gunicorn", "uwsgi", "tomcat", "node", "nodejs"},
	"ctx:shell-binaries":           {"sh", "bash", "dash", "zsh", "ksh", "ash"},
	"ctx:download-client-binaries": {"curl", "wget"},
	"ctx:payload-path-prefixes":    {"/dev/shm/", "/tmp/.sysarmor-attack/", "/var/tmp/.sysarmor-attack/", "/var/lib/app/plugins/"},
	"ctx:credential-path-prefixes": {"/root/.ssh/", "/home/", "/var/run/secrets/", "/run/secrets/", "/etc/kubernetes/"},
	"ctx:trusted-admin-binaries":   {"/usr/bin/vim", "/usr/bin/vi", "/usr/bin/nano"},
	"ioc:c2-download-port-feed":    {"8080"},
}

func builtinContentValues(ref string) []string {
	return append([]string(nil), builtinContextValues[ref]...)
}
