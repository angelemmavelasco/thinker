### Cuenta DEMO

Puedes probar las funcionalidades de thinker al ingresar con una cuenta demo, para ello
es necesario ingresar con un usuario y contraseña preestablecidos.
> * Usuario: thinker.demo@gmail.com
> * Contraseña: thinker1234

Este usuario cuenta con rol demo, por lo que solo es posible ver en primera instancia guías de usuario y soporte genera, además de descripciones de productos relacionados con la plataforma.

Si quieres generar y ver tus propias ideas, no existe restricción para dicho rol y usuario.

### Guía de usuario 

**Qué es Thinker**

Thinker es un espacio de conocimiento colaborativo que transforma notas y documentos en una red de ideas conectadas, respetando el alcance de acceso de cada persona y equipo. En lugar de ordenar archivos en carpetas manuales, la plataforma analiza cada texto, extrae conceptos clave y enlaza únicamente la información que pertenece al mismo ámbito de acceso.

**Flujo de uso**

Acceso: usuario / google

↓

Mis ideas: crear / subir archivo

↓

Síntesis, extracción de términos clave y tópicos

↓

→ Red: explorar grafo y relaciones creadas

→ Pregunta a thinker: consultar red y conexiones

→ Temas e índices: Temas principales y subtemas

**Cómo iniciar sesión**

• Abre Thinker y selecciona Iniciar sesión, o Crear cuenta si aún no tienes usuario.

• Si te registras con correo y contraseña, abre el enlace de confirmación que llega a tu correo en los siguientes quince minutos. 

• Si prefieres acceso directo, selecciona Continuar con Google.

Al entrar verás la barra superior con acceso a Red, Mis ideas y Temas e índices. Si tu cuenta tiene privilegios de Superusuario, verás además la etiqueta Super junto a tu nombre y el acceso a Usuarios y roles.

> Si acabas de registrarte por primera vez y no ves notas compartidas de otros equipos, es el comportamiento esperado: un superusuario debe asignarte tus roles correspondientes.

**Cómo crear notas y subir documentos**

• Entra a Mis ideas y pulsa Nueva idea.

• Escribe un título y redacta el contenido en el editor Markdown, donde puedes cambiar entre escribir, dividir pantalla o ver vista previa.

• Para subir un archivo, pulsa Adjuntar archivo y elige un documento `.md`, `.txt`, `.pdf`, `.docx` o `.csv`.

• Si subes texto plano o Markdown puedes usar Cargar en el editor antes de guardarlo, y si dejas el título vacío se usará el nombre del archivo.

• (Opcional) Abre la sección Visibilidad y permisos por rol debajo del editor para elegir qué roles tendrán permiso de Solo ver o Ver y editar. Si no seleccionas ninguno, la nota se guardará como Privada.

• Pulsa Guardar idea para enviar el contenido al análisis automático. Al guardarse, los textos largos se dividen en fragmentos y pasan brevemente por el estado Borrador antes de quedar en Procesada.

En el listado de Mis ideas cada tarjeta muestra sus etiquetas de visibilidad (Privada o los roles con Ver / Editar). Al abrir cualquier idea accesible puedes revisar su síntesis, el texto original, autor, quién realizó la última edición, términos extraídos y conexiones visibles; según tus permisos podrás editarla, volverla a analizar, eliminarla o reconfigurar sus roles.

**Cómo funcionan los roles (qué es por defecto y qué debe ser explícito)** 

Thinker trabaja con dos niveles globales (Superusuario y Usuario estándar) combinados con roles dinámicos creados por la administración (por ejemplo: Ingeniería, Ventas, Dirección).

_Lo que ocurre por defecto (sin configurar nada extra)_

• Nuevos usuarios: Toda cuenta nueva (registro por correo o Google) se crea como usuario estándar y sin roles asignados. 

• Nuevas notas: Toda idea nueva nace Privada. Solo su autor y los superusuarios pueden verla, editarla, eliminarla o compartirla. 

• Notas heredadas/sin roles: Si una nota no tiene permisos de rol registrados, permanece privada para su autor y superusuarios. 

• Documentos fragmentados: Al crear una nota larga dividida en fragmentos o actualizar sus permisos en el detalle, los permisos de rol se replican automáticamente a todos los fragmentos del mismo documento. 

• Temas e índices: Los usuarios estándar ven únicamente los temas principales y subtemas donde exista al menos una nota accesible para ellos. No ven categorías vacías ni categorías pertenecientes solo a otros roles. 

• Aislamiento de enlaces: Thinker nunca conecta automáticamente una nota privada tuya con notas de otras personas si no comparten un rol.

_Lo que debes configurar de forma explícita_


• Creación de roles: Los roles no existen predefinidos; un superusuario debe crearlos en Usuarios y roles → Roles (nombre y descripción opcional; el identificador slug se genera automáticamente). 

• Asignación de roles a usuarios: Un superusuario debe asignar explícitamente uno o más roles a cada usuario (al crearlo en Nuevo usuario o mediante Gestionar acceso en Usuarios asociados). 

• Compartir una nota con un rol: El autor de la nota o un superusuario debe elegir explícitamente en cada rol entre:
> * Sin acceso (valor por defecto).
> * Solo ver: Permite leer la nota, verla en Red, consultarla en Pregunta a Thinker y verla en Temas e índices.
> * Ver y editar: Todo lo anterior, más permiso para Editar idea y Volver a analizar (registrando al editor como último modificador).
> * Privilegios de superusuario: Conceder o retirar el rol de superusuario se hace explícitamente en Usuarios y roles. 

_Matriz rápida de capacidades_

| Acción | Autor de la nota | Rol con `Solo ver` | Rol con `Ver y editar` | Superusuario |
| --- | :---: | :---: | :---: | :---: |
| Ver nota, grafo y usar en Pregunta a thinker | Sí | Sí | Sí | Sí (todas) |
| Editar contenido y reanalizar | Sí | No | Sí | Sí |
| Eliminar nota o fragmento | Sí | No | No | Sí |
| Cambiar permisos por rol | Sí | No | No | Sí |
| Administrar temas y subtemas | No | No | No | Sí |
| Gestionar usuarios y roles | No | No | No | Sí |

**Cómo explorar la red de ideas y conexiones** 

• Entra a Red desde el menú superior para ver el mapa visual de tu conocimiento. 

• Identifica los elementos por color:
> * Morado: temas maestros
> * Cian: conceptos agrupadores
> * Azul: ideas
> * Ámbar: palabras clave
> * Líneas cian: afinidad semántica
> * Líneas verdes: conceptos en común 

• Haz clic en cualquier nodo para centrar la vista, resaltar sus vecinos y abrir el panel Detalle de selección con su contenido y enlaces directos. 

• Usa Ocultar temas para ver solo las ideas o Centrar red para reencuadrar el mapa.

**Cómo hacer preguntas**



• Dentro de Red, abre el panel Pregunta a Thinker. 

• Escribe tu consulta en el cuadro inferior y presiona Enter o pulsa Preguntar. 

• Lee la respuesta generada exclusivamente a partir de las notas y conexiones a las que tienes permiso de lectura. 

• Haz clic en cualquiera de las Fuentes y conexiones consultadas que aparecen debajo para ubicar e inspeccionar esa nota en el grafo.

**Cómo organizar temas** 

• Entra a Temas e índices para consultar tus notas agrupadas en temas principales y subtemas. 

• Si eres usuario estándar: Verás en modo lectura los temas y subtemas derivados de tus notas propias y compartidas. 

• Si eres superusuario: Tendrás habilitados los controles de administración:
> * Activar o pausar Organización automática según prefieras que Thinker cree categorías nuevas por su cuenta o respete solo las actuales.
> * Crear categorías manuales con Nuevo tema principal o Subtema, fijar un tema (Fijar / Liberar) para evitar cambios automáticos, eliminar categorías o usar Mover para reubicar una nota en otro subtema.

**Cómo administrar usuarios y roles (solo superusuarios)** 

• Entra a Usuarios y roles desde la barra superior. 

• En Roles puedes crear nuevos roles (por ejemplo Producto, Legal, Soporte), editar su nombre/descripción o eliminarlos. Al ver la tabla verás cuántos usuarios y cuántas notas usan cada rol; si eliminas un rol, Thinker lo retira de usuarios y notas y recalcula las conexiones afectadas en el grafo. 

• En Usuarios asociados puedes ver el nivel (Superusuario o Estándar) y los roles de cada cuenta, y pulsar Gestionar acceso para modificar sus roles o privilegios. 

• En Nuevo usuario puedes dar de alta cuentas locales verificadas asignando desde el inicio sus roles y si será superusuario.

**Cómo se maneja la seguridad en Thinker**

_Autenticación y vigencia de sesiones_

Al iniciar sesión se emiten un token de acceso firmado, encriptado y con vigencia de 15 minutos y uno adicional que garantiza rotación y almacenamiento seguro.

El menú superior incorpora Salir (cierra la sesión actual) y el botón de encendido para cerrar todas las sesiones activas, que revoca todos los tokens de acceso.

_Evaluación de permisos en tiempo real y doble barrera_

En cada petición protegida, siempre se valida la versión de token y recarga los roles actuales del usuario y su estado de superusuario. No se confía en los roles que se guardan en los tokens de acceso. Las rutas administrativas están protegidas tanto por middleware como por validación directa en cada servicio.

Cuando un usuario no tiene acceso a una idea, el servidor responde con No encontrada (404) en lugar de revelar que la nota existe.

_Protección contra degradación accidental o bloqueo administrativo_

Un superusuario no puede quitarse a sí mismo el privilegio de superusuario.

El sistema impide retirar el privilegio al último superusuario activo para evitar dejar la plataforma sin administración.

Cuando a un superusuario se le retira ese privilegio, el sistema revoca todos sus tokens, cerrando sus sesiones activas al instante.

### Buenas prácticas: cómo documentar y preguntar con precisión y seguridad 

Thinker es un asistente agnóstico al dominio, responde exclusivamente a partir de la documentación indexada a la que tu usuario tiene acceso. Si un dato no está respaldado en tus notas o no tienes permiso para verlo, no lo inventará ni lo completará con suposiciones externas.
> Thinker utiliza modelos de lenguaje y búsquedas semánticas, por lo que puede cometer errores.

**Cómo redactar notas y subir documentos**

_Usa títulos claros y contexto completo_

Nombra explícitamente el tema, área, producto o regla desde el título y los primeros párrafos. Evita frases ambiguas ("como vimos ayer", "aplica igual que el otro"), ya que los documentos largos se dividen en fragmentos independientes durante el análisis.

_Estructura en Markdown_

Organiza el contenido con encabezados (#, ##), listas y reglas explícitas. Así la síntesis, los conceptos clave y las relaciones del grafo serán mucho más precisos.

_Elige el formato adecuado_

Puedes subir .md, .txt, .pdf, .docx o .csv. Si subes .md o .txt, usa Cargar en el editor para revisar o limpiar encabezados innecesarios antes de guardar. En PDF, asegúrate de que contenga texto seleccionable (no solo imagen escaneada).

_Define la visibilidad desde el inicio_

Toda nota se guarda como Privada por defecto (solo tú y superusuarios). Si contiene información operativa para un equipo, selecciona explícitamente qué roles tienen Solo ver o Ver y editar en Visibilidad y permisos por rol.

_Protege datos sensibles_

No guardes contraseñas, llaves privadas ni tokens dentro de notas compartidas por rol; mantén la información restringida únicamente en los roles que realmente la necesitan (Thinker no conecta ni mezcla contexto entre roles distintos). 

_Verifica el procesamiento_

Espera unos segundos a que la nota pase de Borrador a Procesada y revisa en su detalle que la síntesis y las palabras clave reflejen lo importante.

