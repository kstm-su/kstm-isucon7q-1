<?php

$pdo = new PDO('mysql:host=localhost;dbname=isubata;charset=utf8','root','',
array(PDO::ATTR_EMULATE_PREPARES => false));

$stmt = $pdo->query("SELECT name, data FROM image");
while($row = $stmt -> fetch(PDO::FETCH_ASSOC)){
  file_put_contents($row['name'], $row['data']);
}
