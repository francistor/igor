# Pistas para la implementación de EAP AKA

El AuC por un lado, y el módulo USIM por otro deben ser capaces de generar quintupletas a partir de un RAND. Hay múltiples algoritmos para hacer eso:
- En el Nokia AAA, el algoritmo se especifica en una clase que se pasa en el parámetro -akaclass en aaa-rt
- Uno de los posibles algoritmos es Milenage, especificado en 3GPP PS 35.206
- Aparentemente, además de la clave compartida con el cliente K, se necesitan dos constantes adicionales, OP, de la que sí se comenta algo en la doc del Nokia AAA (parámetro -akaconfig) y el AMF (Authentication Mangament Field), que no sé de dónde se saca.
- El proyecto de github wmnsk/milenage lo implementa

El AAA recibe la quintupleta y ejectuta una derivación de claves
- Genera la MK como un hash de (Identidad+CK+IK)
- Esta MK se pasa por un generador de números aleatorios, especificado en RFC4187 apéndice A, y de ahí se sacan
    - clave de cifrado y clave de encriptación para los mensajes EAP
    - MSK y EMSK

El cliente USIM debe ser capaz de generar el mismo material partiendo del RAND solamente, que es lo que recibirá del AAA. El sinónimo e identidad para reautorización rápida vendrán encriptados con las claves derivadas.