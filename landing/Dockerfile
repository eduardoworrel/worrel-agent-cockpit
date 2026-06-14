# Imagem não-privilegiada: roda como não-root e escuta em 8080 — necessário no
# GuaraCloud, que executa containers como não-root com filesystem read-only
# (o nginx:alpine padrão exige root p/ bindar :80 e escrever em /var/cache,/var/log).
FROM nginxinc/nginx-unprivileged:alpine
COPY index.html styles.css /usr/share/nginx/html/
EXPOSE 8080
