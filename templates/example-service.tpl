upstream example_service {
    {% for server in servers %}
    server {{ server.Host }}:{{ server.Port}} {% if server.Backup %}backup{% else %}{% if server.Weight %}weight={{ server.Weight }} {% endif %}slow_start=3s max_fails=10 fail_timeout=5{% endif %};
    {% endfor %}
}

server {
    listen       9002;

    gzip          off;
    access_log    off;
    server_tokens off;
    log_not_found off;

    location {
        proxy_pass http://example_service;
        proxy_http_version 1.1;
        proxy_next_upstream error timeout http_502;
    }
}