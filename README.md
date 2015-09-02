# docker-gateway

Stupid simple reverse proxy for Docker

### Using with nginx

```
http {
  upstream docker_gateway_local {
    server 127.0.0.1:2377;
  }

  server {
    listen 80;
    server_name *.docker.dev;

    location / {
      proxy_set_header Host $http_host;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_pass http://docker_gateway_local;
      proxy_redirect off;
    }
  }
}
```