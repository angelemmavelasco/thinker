Analiza la nota o cláusula y responde únicamente con JSON válido, sin bloques markdown adicionales, usando esta estructura:
{
  "restructured_note": "Síntesis clara y organizada de la nota en máximo 3 oraciones",
  "key_entities": ["Concepto clave 1", "Concepto clave 2"],
  "target_moc_code": "MOC_NOMBRE_DOMINIO",
  "target_moc_title": "Nombre del dominio temático maestro",
  "target_moc_desc": "Breve descripción del alcance temático",
  "proposed_hub_title": "Concepto o regla sintética general a la que pertenece",
  "hub_summary": "Resumen ejecutivo de la regla en 1 o 2 oraciones",
  "search_aliases": ["termino informal 1", "termino informal 2"]
}

Reglas:
1. Extrae entre 3 y 7 conceptos clave cortos en "key_entities".
2. "target_moc_code" debe ser un código en mayúsculas con prefijo MOC_ (ej: MOC_CANALES, MOC_FARMACOS, MOC_FISCAL, MOC_CREDITO).
3. "proposed_hub_title" debe ser el concepto normativo agrupador (ej: "Restricciones por Giro Comercial", "Sensibilidad Térmica y Manejo de Frío").
4. "search_aliases" debe contener entre 2 y 5 palabras coloquiales o situaciones prácticas donde un usuario buscaría esto (ej: ["estética", "peluquería", "refrigerador", "hielo"]).
5. No inventes información no sustentada en el texto.
