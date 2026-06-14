# Imagem não-privilegiada (não-root, uid 101) + config auto-contida que não
# escreve em /var — necessário no GuaraCloud, que roda containers não-root com
# filesystem read-only. Logs vão p/ stdout/stderr, pid e temp p/ /tmp, porta 8080.
FROM nginxinc/nginx-unprivileged:alpine
COPY nginx.conf /etc/nginx/nginx.conf
COPY index.html styles.css /usr/share/nginx/html/
EXPOSE 8080
