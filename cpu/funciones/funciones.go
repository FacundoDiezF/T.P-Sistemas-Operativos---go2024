package funciones

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/sisoputnfrba/tp-golang/memoria/funciones"
	"github.com/sisoputnfrba/tp-golang/utils/config"
	"github.com/sisoputnfrba/tp-golang/utils/structs"
)

//----------------------( VARIABLES )----------------------\\

// Contiene el pid del proceso que dispatch mandó a ejecutar (se usa para que el handler de la interrupción pueda chequear que el pid del proceso que mandó la interrupción sea el mismo que el pid del proceso que está en ejecución)
var PidEnEjecucion uint32

var HayInterrupcion bool = false

var RegistrosCPU structs.RegistrosUsoGeneral

var ConfigJson config.Cpu

// Es global porque la uso para "depositar" el motivo de desalojo del proceso (que a excepción de EXIT, es traído por una interrupción)
var MotivoDeDesalojo string

// ----------------------( TLB )----------------------\\

// TLB
// Estructura de la TLB.
// ? La página es el key, y el valor es un struct con el marco y el pid.
type TLB map[uint32]struct {
	Marco uint32
	Pid   uint32
}

// Valida si el TLBA está lleno.
func (tlb TLB) Full() bool {
	return len(tlb) == ConfigJson.Number_Felling_tlb
}

// Hit or miss? I guess they never miss, huh?
func (tlb TLB) Hit(pagina uint32) (uint32, bool) {
	strct, encontrado := tlb[pagina]
	return strct.Marco, encontrado
}

// ----------------------( MMU )----------------------\\

// TODO: Probar
func TraduccionMMU(pid uint32, direccionLogica int, tlb TLB) (uint32, bool) {

	// Obtiene la página y el desplazamiento de la dirección lógica
	numeroDePagina, desplazamiento := ObtenerPaginayDesplazamiento(direccionLogica)

	// Obtiene el marco de la página
	marco, encontrado := ObtenerMarco(PidEnEjecucion, uint32(numeroDePagina), tlb)

	// Si no se encontró el marco, se devuelve un error
	if !encontrado {
		//? Cómo manejar el caso de un "Page Fault" (si se debe)?
		return 0, false
	}

	// Calcula la dirección física
	direccionFisica := marco*uint32(funciones.ConfigJson.Page_Size) + uint32(desplazamiento)

	return direccionFisica, true
}

// TODO: Probar
func ObtenerPaginayDesplazamiento(direccionLogica int) (int, int) {

	numeroDePagina := int(math.Floor(float64(direccionLogica) / float64(funciones.ConfigJson.Page_Size)))
	desplazamiento := direccionLogica - numeroDePagina*int(funciones.ConfigJson.Page_Size)

	return numeroDePagina, desplazamiento

}

// TODO: probar
// obtiene el marco de la pagina
func ObtenerMarco(pid uint32, pagina uint32, tlb TLB) (uint32, bool) {

	// Busca en la TLB
	marco, encontrado := buscarEnTLB(pagina, tlb)

	// Si no está en la TLB, busca en la tabla de páginas
	if !encontrado {
		marco, encontrado = buscarEnMemoria(pid, pagina)

		//TODO: agregarTLB(pagina, marco, pid, tlb)
		//TODO: manejar caso en donde la TLB no pueda agregar marco (no existe marco en memoria)
	}

	//? Existe la posibilidad de que un marco no sea hallado
	return marco, encontrado
}

func buscarEnTLB(pagina uint32, tlb TLB) (uint32, bool) {
	marco, encontrado := tlb.Hit(pagina)
	return marco, encontrado
}

func buscarEnMemoria(pid uint32, pagina uint32) (uint32, bool) {

	// Crea un cliente HTTP
	cliente := &http.Client{}
	url := fmt.Sprintf("http://%s:%d/memoria/marco", ConfigJson.Ip_Memory, ConfigJson.Port_Memory)

	// Crea una nueva solicitud PUT
	req, err := http.NewRequest("PUT", url, nil)
	if err != nil {
		fmt.Println(err) //! Borrar despues.
		return 0, false
	}

	// Agrega el PID y la PAGINA como params
	q := req.URL.Query()
	q.Add("pid", fmt.Sprint(pid))
	q.Add("pagina", fmt.Sprint(pagina))
	req.URL.RawQuery = q.Encode()

	// Establece el tipo de contenido de la solicitud
	req.Header.Set("Content-Type", "text/plain")

	// Realiza la solicitud al servidor de memoria
	respuesta, err := cliente.Do(req)
	if err != nil {
		fmt.Println(err) //! Borrar despues.
		return 0, false
	}

	// Verifica el código de estado de la respuesta
	if respuesta.StatusCode != http.StatusOK {
		return 0, false
	}

	// Crea un string para almacenar el marco.
	var marco string

	// Decodifica en formato JSON la request.
	err = json.NewDecoder(respuesta.Body).Decode(&marco)
	if err != nil {
		fmt.Println(err) //! Borrar despues.
		return 0, false
	}

	// Convierte el valor de la instrucción a un uint64 bits.
	valorInt64, err := strconv.ParseUint(marco, 10, 32)
	if err != nil {
		fmt.Println("Error:", err)
		return 0, false
	}

	// Disminuye el valor de la instrucción en uno para ajustarlo al índice del slice de instrucciones.
	marcoEncontrado := uint32(valorInt64)

	return uint32(marcoEncontrado), true

}

//----------------------( FUNCIONES CICLO DE INSTRUCCION )----------------------\\

// Ejecuta un ciclo de instruccion.
func EjecutarCiclosDeInstruccion(PCB *structs.PCB) {
	var cicloFinalizado bool = false

	// Itera el ciclo de instrucción si hay instrucciones a ejecutar y no hay interrupciones.
	for !HayInterrupcion && !cicloFinalizado {
		// Obtiene la próxima instrucción a ejecutar.
		instruccion := Fetch(PCB.PID, RegistrosCPU.PC)

		// Decodifica y ejecuta la instrucción.
		DecodeAndExecute(PCB, instruccion, &RegistrosCPU.PC, &cicloFinalizado)
	}
	HayInterrupcion = false // Resetea la interrupción

	// Actualiza los registros de uso general del PCB con los registros de la CPU.
	PCB.RegistrosUsoGeneral = RegistrosCPU
}

// Trae de memoria las instrucciones indicadas por el PC y el PID.
func Fetch(PID uint32, PC uint32) string {

	// Convierte el PID y el PC a string
	pid := strconv.FormatUint(uint64(PID), 10)
	pc := strconv.FormatUint(uint64(PC), 10)

	// Crea un cliente HTTP
	cliente := &http.Client{}
	url := fmt.Sprintf("http://%s:%d/instrucciones", ConfigJson.Ip_Memory, ConfigJson.Port_Memory)

	// Crea una nueva solicitud GET
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ""
	}

	// Agrega el PID y el PC como params
	q := req.URL.Query()
	q.Add("PID", pid)
	q.Add("PC", pc)
	req.URL.RawQuery = q.Encode()

	// Establece el tipo de contenido de la solicitud
	req.Header.Set("Content-Type", "text/plain")

	// Realiza la solicitud al servidor de memoria
	respuesta, err := cliente.Do(req)
	if err != nil {
		return ""
	}

	// Verifica el código de estado de la respuesta
	if respuesta.StatusCode != http.StatusOK {
		return ""
	}

	// Lee el cuerpo de la respuesta
	bodyBytes, err := io.ReadAll(respuesta.Body)
	if err != nil {
		return ""
	}

	// Retorna las instrucciones obtenidas como una cadena de texto
	return string(bodyBytes)
}

// Ejecuta las instrucciones traidas de memoria.
func DecodeAndExecute(PCB *structs.PCB, instruccion string, PC *uint32, cicloFinalizado *bool) {

	// Mapa de registros para acceder a los registros de la CPU por nombre
	var registrosMap8 = map[string]*uint8{
		"AX": &RegistrosCPU.AX,
		"BX": &RegistrosCPU.BX,
		"CX": &RegistrosCPU.CX,
		"DX": &RegistrosCPU.DX,
	}

	var registrosMap32 = map[string]*uint32{
		"EAX": &RegistrosCPU.EAX,
		"EBX": &RegistrosCPU.EBX,
		"ECX": &RegistrosCPU.ECX,
		"EDX": &RegistrosCPU.EDX,
	}

	// Parsea las instrucciones de la cadena de instrucción
	variable := strings.Split(instruccion, " ")

	// Imprime la instrucción y sus parámetros
	fmt.Println("Instruccion: ", variable[0], " Parametros: ", variable[1:])

	// Switch para determinar la operación a realizar según la instrucción
	switch variable[0] {
	case "SET":
		Set(variable[1], variable[2], registrosMap8, PC)

	case "SUM":
		Sum(variable[1], variable[2], registrosMap8)

	case "SUB":
		Sub(variable[1], variable[2], registrosMap8)

	case "JNZ":
		Jnz(variable[1], variable[2], PC, registrosMap8)

	case "RESIZE":
		resize(variable[1])

	case "IO_GEN_SLEEP":
		*cicloFinalizado = true
		PCB.Estado = "BLOCK"
		go IoGenSleep(variable[1], variable[2], registrosMap8, PCB.PID)

	case "IO_STDIN_READ":
		*cicloFinalizado = true
		PCB.Estado = "BLOCK"
		IO_STDIN_READ(variable[1], variable[2], variable[3], registrosMap8, registrosMap32, PCB.PID)

	case "EXIT":
		*cicloFinalizado = true
		PCB.Estado = "EXIT"
		MotivoDeDesalojo = "EXIT"

		return

	default:
		fmt.Println("------")
	}

	// Incrementa el Program Counter para apuntar a la siguiente instrucción
	*PC++
}

//----------------------( FUNCIONES DE INSTRUCCIONES )----------------------\\

// Asigna al registro el valor pasado como parámetro.
func Set(reg string, dato string, registroMap map[string]*uint8, PC *uint32) {

	// Verifica si el registro a asignar es el PC
	if reg == "PC" {

		// Convierte el valor a un entero sin signo de 32 bits
		valorInt64, err := strconv.ParseUint(dato, 10, 32)

		if err != nil {
			fmt.Println("Dato no valido")
		}

		// Asigna el valor al PC (resta 1 ya que el PC se incrementará después de esta instrucción)
		*PC = uint32(valorInt64) - 1
		return
	}

	// Obtiene el puntero al registro del mapa de registros
	registro, encontrado := registroMap[reg]
	if !encontrado {
		fmt.Println("Registro invalido")
		return
	}

	// Convierte el valor de string a entero
	valor, err := strconv.Atoi(dato)

	if err != nil {
		fmt.Println("Dato no valido")
	}

	// Asigna el nuevo valor al registro
	*registro = uint8(valor)
}

// Suma al Registro Destino el Registro Origen y deja el resultado en el Registro Destino.
func Sum(reg1 string, reg2 string, registroMap map[string]*uint8) {

	// Verifica si existen los registros especificados en la instrucción.
	registro1, encontrado := registroMap[reg1]
	if !encontrado {
		fmt.Println("Registro invalido")
		return
	}

	registro2, encontrado := registroMap[reg2]
	if !encontrado {
		fmt.Println("Registro invalido")
		return
	}

	// Suma el valor del Registro Origen al Registro Destino.
	*registro1 += *registro2
}

// Resta al Registro Destino el Registro Origen y deja el resultado en el Registro Destino.
func Sub(reg1 string, reg2 string, registroMap map[string]*uint8) {

	// Verifica si existen los registros especificados en la instrucción.
	registro1, encontrado := registroMap[reg1]
	if !encontrado {
		fmt.Println("Registro invalido")
		return
	}

	registro2, encontrado := registroMap[reg2]
	if !encontrado {
		fmt.Println("Registro invalido")
		return
	}

	// Resta el valor del Registro Origen al Registro Destino.
	*registro1 -= *registro2
}

// Si el valor del registro es distinto de cero, actualiza el PC al numero de instruccion pasada por parametro.
func Jnz(reg string, valor string, PC *uint32, registroMap map[string]*uint8) {

	// Verifica si existe el registro especificado en la instrucción.
	registro, encontrado := registroMap[reg]
	if !encontrado {
		fmt.Println("Registro invalido")
		return
	}

	// Convierte el valor de la instrucción a un uint64 bits.
	valorInt64, err := strconv.ParseUint(valor, 10, 32)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Disminuye el valor de la instrucción en uno para ajustarlo al índice del slice de instrucciones.
	nuevoValor := uint32(valorInt64) - 1

	// Si el valor del registro es distinto de cero, actualiza el PC al nuevo valor.
	if *registro != 0 {
		*PC = nuevoValor
	}
}

// TODO: Probar
func resize(tamañoEnBytes string) string {
	// Convierte el PID y el PC a string
	pid := strconv.FormatUint(uint64(PidEnEjecucion), 10)

	// Crea un cliente HTTP
	cliente := &http.Client{}
	url := fmt.Sprintf("http://%s:%d/memoria/resize", ConfigJson.Ip_Memory, ConfigJson.Port_Memory)

	// Crea una nueva solicitud GET
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ""
	}

	// Agrega el PID y el PC como params
	q := req.URL.Query()
	q.Add("pid", pid)
	q.Add("size", tamañoEnBytes)
	req.URL.RawQuery = q.Encode()

	// Establece el tipo de contenido de la solicitud
	req.Header.Set("Content-Type", "text/plain")

	// Realiza la solicitud al servidor de memoria
	respuesta, err := cliente.Do(req)
	if err != nil {
		return ""
	}

	// Verifica el código de estado de la respuesta
	if respuesta.StatusCode != http.StatusOK {
		return ""
	}

	// Lee el cuerpo de la respuesta
	bodyBytes, err := io.ReadAll(respuesta.Body)
	if err != nil {
		return ""
	}

	// Retorna las instrucciones obtenidas como una cadena de texto
	return string(bodyBytes)

}

// Envía una request a Kernel con el nombre de una interfaz y las unidades de trabajo a multiplicar.
func IoGenSleep(nombreInterfaz string, unitWorkTimeString string, registroMap map[string]*uint8, PID uint32) {

	// Convierte el tiempo de trabajo de la unidad de cadena a entero.
	unitWorkTime, err := strconv.Atoi(unitWorkTimeString)
	if err != nil {
		return
	}

	//Creo estructura de request
	var requestEjecutarInstuccion = structs.RequestEjecutarInstruccionIO{
		PidDesalojado:  PID,
		NombreInterfaz: nombreInterfaz,
		Instruccion:    "IO_GEN_SLEEP",
		UnitWorkTime:   unitWorkTime,
	}

	//Convierto request a JSON
	body, err := json.Marshal(requestEjecutarInstuccion)
	if err != nil {
		return
	}

	// Envía la solicitud de ejecucion a Kernel
	config.Request(ConfigJson.Port_Kernel, ConfigJson.Ip_Kernel, "POST", "instruccionIO", body)

}

// Ejemplo de uso: IO_STDIN_READ Int2 EAX AX
// Envía una request a Kernel con el nombre de una interfaz y las unidades de trabajo a multiplicar.
func IO_STDIN_READ(nombreInterfaz string, regDir string, regTamaño string, registroMap8 map[string]*uint8, registroMap32 map[string]*uint32, PID uint32) {

	// Verifica si existe el registro especificado en la instrucción.
	registroDireccion, encontrado := registroMap32[regDir]
	if !encontrado {
		fmt.Println("Registro invalido")
		return
	}

	// Verifica si existe el registro especificado en la instrucción.
	registroTamaño, encontrado := registroMap8[regTamaño]
	if !encontrado {
		fmt.Println("Registro invalido")
		return
	}

	// Creo estructura de request
	var requestEjecutarInstuccion = structs.RequestEjecutarInstruccionIO{
		PidDesalojado:     PID,
		NombreInterfaz:    nombreInterfaz,
		Instruccion:       "IO_STDIN_READ",
		RegistroDireccion: *registroDireccion,
		RegistroTamaño:    *registroTamaño,
	}

	// Convierto request a JSON
	body, err := json.Marshal(requestEjecutarInstuccion)
	if err != nil {
		return
	}

	// Envía la solicitud de ejecucion a Kernel
	config.Request(ConfigJson.Port_Kernel, ConfigJson.Ip_Kernel, "POST", "instruccionIO", body)

	/*
	   El texto de la respuesta se va a guardar en la memoria a partir de la
	   DIRECCION FISICA indicada en la petición que recibió por parte del Kernel.
	*/

}

// func IO_STDOUT_READ(){

// }

// func MOV_IN(){

// }

// func MOV_OUT(){

// }

// func COPY_STRING(){

// }
