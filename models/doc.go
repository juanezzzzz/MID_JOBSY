
// package models define el contrato que ve el frontend Angular.
// Estos tipos son un calco de src/app/models/*.ts de Jobsy_Front. El
// front ya esta escrito sobre ellos, asi que aqui mandan ellos, no las tablas.
// Toda la distancia entre esta forma y la de PostgresSQL la cubre el mid.

// Diferencias deliberadas respecto a la base de datos:
//
// - Los ids viajan como string. La BD usa SERIAL (int). El front tipa
//   id: string en todos sus modelos.
// - Los enums viajan como texto ('por_horas'). La DB guarda claves foraneas
//  a catalogo.*. La traduccion vive en el paquete catalogo.
// - tamano y gama van separados: tamano de 3 valores + esLujo booleano.
//  La DB mete 'lujoso' dentro de tamano_inmueble, que confunde magnitud con
//  gama. Mantenemos la forma del front porque es la correcta.

package models
