// Package models define el contrato que ve el frontend Angular.
//
// Estos tipos son un calco de src/app/core/models/*.ts de Jobsy_Front. El
// front ya esta escrito contra ellos, asi que aqui mandan ellos, no las tablas.
// Toda la distancia entre esta forma y la de PostgreSQL la cubre el mid.
//
// Diferencias deliberadas respecto a la base de datos:
//
//   - Los ids viajan como string. La BD usa SERIAL (int). El front tipa
//     id: string en todos sus modelos.
//   - Los enums viajan como texto ('por_horas'). La BD guarda claves foraneas
//     a catalogo.*. La traduccion vive en el paquete catalogo.
//   - Tamano y gama van separados: tamano de 3 valores + esLujo booleano.
//     La BD mete 'lujoso' dentro de tamano_inmueble, que confunde magnitud con
//     gama. Mantenemos la forma del front porque es la correcta.
package models
