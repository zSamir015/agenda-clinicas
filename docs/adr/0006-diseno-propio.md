# 0006. Sistema de diseño y calendario propios

Estado: aceptada (2026-11-13)

## Contexto

El producto necesita identidad visual propia. Las librerías de componentes con estilo y FullCalendar imponen su apariencia, y la vista por recurso de FullCalendar es de pago.

## Decisión

Tokens y componentes propios sobre primitivas de Radix sin estilos, y un calendario de agenda propio.

## Consecuencias

- La fase 8 crece entre 15 y 20 horas y la fase 4 incluye el calendario (12 a 15 horas).
- La accesibilidad de los componentes complejos viene de Radix; la del calendario se prueba aparte.
