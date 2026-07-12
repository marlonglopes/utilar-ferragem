# Headers comuns de proxy (incluído por cada location do nginx.prod.conf).
proxy_http_version 1.1;
proxy_set_header Host              $host;
proxy_set_header X-Real-IP         $remote_addr;
proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header X-Request-Id      $request_id;
proxy_connect_timeout 5s;
proxy_read_timeout    30s;
