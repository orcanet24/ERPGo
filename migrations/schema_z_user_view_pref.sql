-- Vistas y preferencias por usuario.
--
-- "Plantilla" aquí significa la disposición de una pantalla: qué módulos se
-- ven, cómo se ordenan, qué filtros quedan puestos, cuántas filas por página.
-- No es una plantilla de informe (eso vive en report_template, que describe
-- columnas y filtros de un reporte del motor F5) ni un ajuste de la empresa
-- (eso es web_setting): es la forma en que una persona concreta mira la
-- pantalla. Que sea de la persona y no de la empresa es justamente el punto.
--
-- DENTRO DE LA BASE DE LA EMPRESA, y no en la de control, por una razón
-- práctica: la misma persona puede trabajar en dos empresas y en cada una tener
-- menús y filtros distintos. Al vivir en el archivo de la empresa, esa
-- separación sale gratis y, sobre todo, un respaldo de la empresa se lleva por
-- delante las preferencias de quienes trabajan en ella. Si vivieran en
-- erpgo.db, restaurar "la empresa del lunes" dejaría a su gente con las vistas
-- de quien las tenía el martes.
--
-- `name` vacío = la vista que el usuario tiene activa ahora mismo. Con nombre =
-- una plantilla guardada ("Solo lo pendiente", "Resumen del día"). Así se
-- cubren las dos cosas que se piden con una sola tabla y sin duplicar el
-- estado: lo que se está viendo y lo que se decidió guardar para después.
--
-- `payload` es JSON opaco para el servidor. Guardarlo como texto y no como
-- columnas permite añadir una preferencia nueva sin migración, y evita que el
-- sistema afirme entender algo que en realidad no entiende.
CREATE TABLE IF NOT EXISTS `user_view_pref` (
  `username`   varchar(60)  NOT NULL DEFAULT '',
  `view_key`   varchar(80)  NOT NULL DEFAULT '',
  `name`       varchar(80)  NOT NULL DEFAULT '',
  `payload`    mediumtext    NOT NULL,
  `updated_at` varchar(20)  NOT NULL DEFAULT '',
  PRIMARY KEY (`username`, `view_key`, `name`)
);

CREATE INDEX idx_user_view_pref_screen ON `user_view_pref`(`view_key`);
