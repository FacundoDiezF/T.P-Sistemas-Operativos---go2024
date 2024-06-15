package funciones

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/sisoputnfrba/tp-golang/utils/config"
	"github.com/sisoputnfrba/tp-golang/utils/structs"
)

var ConfigJson config.Memoria

// Funciones auxiliares
// Toma de a un archivo a la vez y guarda las instrucciones en un map l
func GuardarInstrucciones(pid uint32, path string, memoriaInstrucciones map[uint32][]string) {
	path = ConfigJson.Instructions_Path + "/" + path
	data := ExtractInstructions(path)
	InsertData(pid, memoriaInstrucciones, data)
}

// Abre el archivo especificado por la ruta 'path' y guarda su contenido en un slice de bytes.
// Retorna el contenido del archivo como un slice de bytes.
func ExtractInstructions(path string) []byte {
	// Lee el archivo
	file, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("Error al leer el archivo de instrucciones")
		return nil
	}

	// Ahora 'file' es un slice de bytes con el contenido del archivo
	return file
}

// insertData inserta las instrucciones en la memoria de instrucciones asociadas al PID especificado
// e imprime las instrucciones guardadas en memoria junto con su PID correspondiente.
func InsertData(pid uint32, memoriaInstrucciones map[uint32][]string, data []byte) {
	// Separar las instrucciones por medio de tokens
	instrucciones := strings.Split(string(data), "\n")
	// Insertar las instrucciones en la memoria de instrucciones
	memoriaInstrucciones[pid] = instrucciones
	// Imprimir las instrucciones guardadas en memoria
	fmt.Println("Instrucciones guardadas en memoria: ")
	for pid, instrucciones := range memoriaInstrucciones {
		fmt.Printf("PID: %d\n", pid)
		for _, instruccion := range instrucciones {
			fmt.Println(instruccion)
		}
		fmt.Println()
	}
}

func AsignarTabla(pid uint32, tablaDePaginas map[uint32]structs.Tabla) {
	tablaDePaginas[pid] = structs.Tabla{}
}

func BuscarMarco(pid uint32, pagina uint32, tablaDePaginas map[uint32]structs.Tabla) string {
	if len(tablaDePaginas[pid]) <= int(pagina) {
		return ""
	}

	marco := tablaDePaginas[pid][pagina]

	marcoStr := strconv.Itoa(marco)

	return marcoStr
}

func ObtenerPagina(pid uint32, direccionFisica uint32, tablaDePaginas map[uint32]structs.Tabla) int {

	marco := math.Floor(float64(direccionFisica) / float64(ConfigJson.Page_Size))

	for i := range tablaDePaginas[pid] {

		marcoActual := tablaDePaginas[pid][i]

		if uint32(marcoActual) == uint32(marco) {

			return i

		}
	}

	return -1
}

func tableHasNext(pid uint32, pagina uint32, tablaDePaginas map[uint32]structs.Tabla) bool {
	return len(tablaDePaginas[pid])-1 > int(pagina)
}

// Verifica si la pagina aun tiene espacio en memoria
func endOfPage(direccionFisica uint32) bool {
	//Si la direccion es multiplo del tamaño de pagina, es el fin de la pagina
	return direccionFisica%uint32(ConfigJson.Page_Size) == 0
}

func LiberarMarcos(marcosALiberar []int, bitMap []bool) {
	for _, marco := range marcosALiberar {
		bitMap[marco] = false
	}
}

func ReasignarPaginas(pid uint32, tablaDePaginas *map[uint32]structs.Tabla, bitMap []bool, size uint32) string {

	lenOriginal := len((*tablaDePaginas)[pid]) //!

	cantidadDePaginas := int(math.Ceil(float64(size) / float64(ConfigJson.Page_Size)))

	//*CASO AGREGAR PAGINAS
	//?Hace falta devolver algo?
	// Itera n cantidad de veces, siendo n la cantidad de paginas a agregar
	//? Funcionan los punteros así?
	for len((*tablaDePaginas)[pid]) < cantidadDePaginas {

		// Por cada página a agregar, si no hay marcos disponibles, se devuelve un error OUT OF MEMORY
		outOfMemory := true

		// Recorre el bitMap buscando un marco desocupado
		for marco, ocupado := range bitMap {
			//?optimizar? (no se si es necesario recorrer todo el bitMap)

			if !ocupado {
				// Guarda en la tabla de páginas del proceso el marco asignado a una página
				(*tablaDePaginas)[pid] = append((*tablaDePaginas)[pid], marco)
				// Marca el marco como ocupado
				bitMap[marco] = true

				// Notifica que por ahora no está OUT OF MEMORY
				outOfMemory = false
			}
		}

		//Si no hubo ningun marco desocupado para la página anterior, devuelve OUT OF MEMORY
		if outOfMemory {
			return "OUT OF MEMORY" //?
			//!OUT OF MEMORY
		}
	}

	//*CASO QUITAR PAGINAS
	//?Hace falta devolver algo?
	if len((*tablaDePaginas)[pid]) > cantidadDePaginas {

		marcosALiberar := (*tablaDePaginas)[pid][cantidadDePaginas:]

		(*tablaDePaginas)[pid] = (*tablaDePaginas)[pid][:cantidadDePaginas]

		LiberarMarcos(marcosALiberar, bitMap)
	}

	fmt.Printf("Se pasó de %d a %d páginas\n", lenOriginal, len((*tablaDePaginas)[pid]))

	return "OK" //?
}

func LeerEnMemoria(pid uint32, tablaDePaginas map[uint32]structs.Tabla, pagina uint32, direccionFisica uint32, byteArraySize int, espacioUsuario *[]byte) ([]byte, string) {

	var dato []byte

	// Itera sobre los bytes del dato recibido.
	for ; byteArraySize > 0; byteArraySize-- {

		// Lee el byte en la dirección física.
		dato = append(dato, (*espacioUsuario)[direccionFisica])

		// Incrementa la dirección
		direccionFisica++

		// Si la siguiente direccion fisica es endOfPage (ya no pertenece al marco en el que estamos escribiendo), hace cambio de página
		if endOfPage(direccionFisica) {
			// Si no se puede hacer el cambio de página, es OUT OF MEMORY
			if !cambioDePagina(&direccionFisica, pid, tablaDePaginas, pagina) {
				return dato, "OUT OF MEMORY"
			}
		}
	}

	return dato, "OK" //?
}

// Escribe en memoria el dato recibido en la dirección física especificada.
func EscribirEnMemoria(pid uint32, tablaDePaginas map[uint32]structs.Tabla, pagina uint32, direccionFisica uint32, datoBytes []byte, espacioUsuario *[]byte) string {

	// Itera sobre los bytes del dato recibido.
	for i := range datoBytes {

		// Escribe el byte en la dirección física.
		(*espacioUsuario)[direccionFisica] = datoBytes[i]

		// Incrementa la dirección
		direccionFisica++

		// Si la siguiente direccion fisica es endOfPage (ya no pertenece al marco en el que estamos escribiendo), hace cambio de página
		if endOfPage(direccionFisica) {
			// Si no se puede hacer el cambio de página, es OUT OF MEMORY
			if !cambioDePagina(&direccionFisica, pid, tablaDePaginas, pagina) {
				return "OUT OF MEMORY"
			}
		}
	}

	return "OK" //?
}

func cambioDePagina(direccionFisica *uint32, pid uint32, tablasDePaginas map[uint32]structs.Tabla, pagina uint32) bool {

	if tableHasNext(pid, pagina, tablasDePaginas) {
		// Cambio la direccion fisica a la primera del siguitabla
		*direccionFisica = uint32(((tablasDePaginas)[pid][pagina+1]) * int(ConfigJson.Page_Size))
		return true
	}
	return false
}
