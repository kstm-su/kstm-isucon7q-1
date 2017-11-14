mysqldumpslow -s t /var/log/mysql/mysql-slow.sql > mysqldumpslow.log.`date "+%s"`
rm /var/log/mysql/mysql-slow.sql
systemctl restart mysql
git add .
git add /etc/mysql/my.cnf
git commit --allow-empty -m "rotate"
