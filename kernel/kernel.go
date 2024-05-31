package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/sisoputnfrba/tp-golang/kernel/funciones"
	"github.com/sisoputnfrba/tp-golang/kernel/logueano"
	"github.com/sisoputnfrba/tp-golang/utils/config"
	"github.com/sisoputnfrba/tp-golang/utils/structs"
)

//*======================================| MAIN |======================================\\

func main() {

	config.Iniciar("config.json", &funciones.ConfigJson)
	funciones.Cont_producirPCB = make(chan int, funciones.ConfigJson.Multiprogramming)
	funciones.Bin_hayPCBenREADY = make(chan int, funciones.ConfigJson.Multiprogramming+1)

	// Configura el logger
	config.Logger("Kernel.log")

	// ======== HandleFunctions ========

	//PLANIFICACION
	http.HandleFunc("PUT /plani", handlerIniciarPlanificacion)
	http.HandleFunc("DELETE /plani", handlerDetenerPlanificacion)

	//PROCESOS
	http.HandleFunc("GET /process/{pid}", handlerEstadoProceso)
	http.HandleFunc("GET /process", handlerListarProceso)

	http.HandleFunc("PUT /process", handlerIniciarProceso)
	http.HandleFunc("DELETE /process/{pid}", handlerFinalizarProceso)

	//ENTRADA SALIDA
	http.HandleFunc("POST /interfazConectada", handlerIniciarInterfaz)
	http.HandleFunc("POST /instruccion", handlerInstrucciones)

	//Inicio el servidor de Kernel
	config.IniciarServidor(funciones.ConfigJson.Port)

}

//*======================================| HANDLERS |======================================\\

//----------------------( PLANIFICACION )----------------------\\

// TODO:Al recibir esta peticion comienza la ejecucion de el planificador de largo plazo (y corto plazo)
func handlerIniciarPlanificacion(w http.ResponseWriter, r *http.Request) {

	fmt.Println("IniciarPlanificacion-------------------------")
	funciones.TogglePlanificador = true

	funciones.OnePlani.Lock()
	go funciones.Planificador()

	w.WriteHeader(http.StatusOK)
}

// TODO:Al recibir esta peticion detiene la ejecucion de el planificador de largo plazo (y corto plazo)
func handlerDetenerPlanificacion(w http.ResponseWriter, r *http.Request) {

	fmt.Printf("DetenerPlanificacion-------------------------")

	funciones.TogglePlanificador = false
	funciones.OnePlani.Unlock()

	w.WriteHeader(http.StatusOK)
}

//----------------------( PROCESOS )----------------------\\

func handlerIniciarProceso(w http.ResponseWriter, r *http.Request) {

	fmt.Println("IniciarProceso-------------------------")

	//----------- RECIBE ---------
	//variable que recibirá la request.
	var request structs.RequestIniciarProceso

	// Decodifica en formato JSON la request.
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		fmt.Println("Error al decodificar request body: ")
		fmt.Println(err)

		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fmt.Printf("Path: %s\n", request.Path)

	//----------- EJECUTA ---------

	// Se crea un nuevo PCB en estado NEW
	var nuevoPCB structs.PCB

	funciones.Mx_ConterPID.Lock()
	nuevoPCB.PID = funciones.CounterPID
	funciones.Mx_ConterPID.Unlock()

	nuevoPCB.Estado = "NEW"

	//----------- Va a memoria ---------
	bodyIniciarProceso, err := json.Marshal(structs.BodyIniciarProceso{PID: nuevoPCB.PID, Path: request.Path})
	if err != nil {
		return
	}

	//Envía el path a memoria para que cree el proceso
	respuesta := config.Request(funciones.ConfigJson.Port_Memory, funciones.ConfigJson.Ip_Memory, "PUT", "process", bodyIniciarProceso)
	if respuesta == nil {
		return
	}

	var respMemoIniciarProceso structs.BodyIniciarProceso
	// Decodifica en formato JSON la request.
	err = json.NewDecoder(respuesta.Body).Decode(&respMemoIniciarProceso)
	if err != nil {
		fmt.Println("Error al decodificar request body")
		return
	}
	//----------------------------

	//Asigna un nuevo valor pid para la proxima response.
	funciones.Mx_ConterPID.Lock()
	funciones.CounterPID++
	funciones.Mx_ConterPID.Unlock()

	//Verifica si puede producir un PCB (por Multiprogramacion)
	funciones.Cont_producirPCB <- 0

	// Si todo es correcto agregamos el PID al PCB
	nuevoPCB.Estado = "READY"

	// Agrega el nuevo PCB a readyQueue
	funciones.AdministrarQueues(nuevoPCB)

	//^ log obligatorio (2/6) (NEW->Ready): Cambio de Estado
	logueano.CambioDeEstado("NEW", nuevoPCB)

	// ----------- DEVUELVE -----------

	respIniciarProceso, err := json.Marshal(respMemoIniciarProceso.PID)
	if err != nil {
		http.Error(w, "Error al codificar el JSON de la respuesta", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(respIniciarProceso)
}

// TODO:
func handlerFinalizarProceso(w http.ResponseWriter, r *http.Request) {

	fmt.Println("DetenerEstadoProceso-------------------------")

	//--------- RECIBE ---------
	pid, error := strconv.Atoi(r.PathValue("pid"))
	if error != nil {
		http.Error(w, "Error al obtener el ID del proceso", http.StatusInternalServerError)
		return
	}

	fmt.Println("PID:", pid)

	//--------- EJECUTA ---------

	//* Busca el Proceso (PID) lo desencola y lo pasa a EXIT (si esta en EXEC, lo interrumpe y lo pasa a EXIT)

	// Envía respuesta (con estatus como header) al cliente
	w.WriteHeader(http.StatusOK)
}

// TODO: Tomar los procesos creados (BLock, Ready y Exec) y devolverlos en una lista
func handlerListarProceso(w http.ResponseWriter, r *http.Request) {

	fmt.Printf("ListarProceso-------------------------")

	//----------- EJECUTA -----------

	//Harcodea una lista de procesos, más adelante deberá ser dinámico
	var listaDeProcesos []structs.ResponseListarProceso = []structs.ResponseListarProceso{
		{PID: 0, Estado: "READY"},
		{PID: 1, Estado: "BLOCK"},
	}

	//----------- DEVUELVE -----------

	//Paso a formato JSON la lista de procesos
	respuesta, err := json.Marshal(listaDeProcesos)

	//Check si hubo algún error al parsear el JSON
	if err != nil {
		http.Error(w, "Error al codificar los datos como JSON", http.StatusInternalServerError)
		return
	}

	// Envía respuesta al cliente
	w.WriteHeader(http.StatusOK)
	w.Write(respuesta)
}

// TODO: Busca el proceso deseado y devuelve el estado en el que se encuentra
func handlerEstadoProceso(w http.ResponseWriter, r *http.Request) {

	fmt.Println("DetenerEstadoProceso-------------------------")

	//--------- RECIBE ---------
	pid, error := strconv.Atoi(r.PathValue("pid"))
	if error != nil {
		http.Error(w, "Error al obtener el ID del proceso", http.StatusInternalServerError)
		return
	}

	fmt.Println("PID:", pid)

	//--------- EJECUTA ---------

	//TODO: Busca en base al pid el proceso en todas las colas (y el map de BLOCK) y devuelvo el estado
	var respIniciarProceso structs.ResponseEstadoProceso = structs.ResponseEstadoProceso{State: "ANASHE"}

	//--------- DEVUELVE ---------
	//Crea una variable tipo Response
	respuesta, err := json.Marshal(respIniciarProceso)

	// Error Handler de la codificación
	if err != nil {
		http.Error(w, "Error al codificar los datos como JSON", http.StatusInternalServerError)
		return
	}

	// Envía respuesta (con estatus como header) al cliente
	w.WriteHeader(http.StatusOK)
	w.Write(respuesta)
}

//----------------------( I/O )----------------------\\

// Recibe una interfazConectada y la agrega al map de interfaces conectadas.
func handlerIniciarInterfaz(w http.ResponseWriter, r *http.Request) {

	fmt.Println("IniciarInterfaz-------------------------")

	// Se crea una variable para almacenar la interfaz recibida en la solicitud.
	var requestInterfaz structs.RequestInterfaz

	// Se decodifica el cuerpo de la solicitud en formato JSON.
	err := json.NewDecoder(r.Body).Decode(&requestInterfaz)

	// Maneja el error en la decodificación.
	if err != nil {
		logueano.ErrorDecode()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Imprime la solicitud
	fmt.Println("Request path:", requestInterfaz)

	//Guarda la interfazConectada en la lista de interfaces conectadas.
	funciones.InterfacesConectadas.Set(requestInterfaz.NombreInterfaz, requestInterfaz.Interfaz)

	// Envía una señal al canal 'hayInterfaz' para indicar que hay una nueva interfaz conectada.

}

// TODO: implementar para los demás tipos de interfaces (cambiar tipos de datos en request y body)
func handlerInstrucciones(w http.ResponseWriter, r *http.Request) {

	// Se crea una variable para almacenar la instrucción recibida en la solicitud.
	var request structs.InstruccionIO

	// Se decodifica el cuerpo de la solicitud en formato JSON.
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		logueano.ErrorDecode()
		return
	}

	// Imprime la solicitud
	fmt.Println("Request path:", request)

	// Busca la interfaz conectada en el mapa de funciones.InterfacesConectadas.
	interfazConectada, encontrado := funciones.InterfacesConectadas.Get(request.NombreInterfaz)
	// Si no se encontró la interfazConectada de la request, se desaloja el structs.
	if !encontrado {
		funciones.DesalojarProceso(request.PidDesalojado, "EXIT")
		fmt.Println("Interfaz no conectada.")
		return
	}

	//Verifica que la instruccion sea compatible con el tipo de interfazConectada.
	isValid := funciones.ValidarInstruccion(interfazConectada.TipoInterfaz, request.Instruccion)

	// Si la instrucción no es compatible, se desaloja el proceso y se marca como "EXIT".
	if !isValid {

		funciones.DesalojarProceso(request.PidDesalojado, "EXIT")
		fmt.Println("Interfaz incompatible.")
		return
	}

	// Agrega el Proceso a la cola de bloqueados de la interfazConectada.
	interfazConectada.QueueBlock = append(interfazConectada.QueueBlock, request.PidDesalojado)
	funciones.InterfacesConectadas.Set(request.NombreInterfaz, interfazConectada)

	// Prepara la interfazConectada para enviarla en el body.
	body, err := json.Marshal(request.UnitWorkTime)

	// Maneja los errores al crear el body.
	if err != nil {
		fmt.Printf("error codificando body: %s", err.Error())
		return
	}

	// Envía la instrucción a ejecutar a la interfazConectada (Puerto).
	respuesta := config.Request(interfazConectada.PuertoInterfaz, "localhost", "POST", request.Instruccion, body)

	// Verifica que no hubo error en la request
	if respuesta == nil {
		return
	}

	// Si la interfazConectada pudo ejecutar la instrucción, pasa el Proceso a READY.
	if respuesta.StatusCode == http.StatusOK {
		// Pasa el proceso a READY y lo quita de la lista de bloqueados.
		funciones.DesalojarProceso(request.PidDesalojado, "READY")
		return
	}
}

//*======================================| FUNC de TESTEO |======================================\\
// !ESTO NO SE MIGRO A NINGUN PAQUETE
// ! TRAS LOS CAMBIOS DUDO QUE FUNCIONEN (29/5/24)

/*
func testConectividad(configJson config.Kernel) {
	fmt.Println("\nIniciar Proceso:")
	funciones.IniciarProceso(configJson, "path")
	funciones.IniciarProceso(configJson, "path")
	funciones.IniciarProceso(configJson, "path")
	funciones.IniciarProceso(configJson, "path")
	fmt.Println("\nFinalizar Proceso:")
	funciones.FinalizarProceso(configJson)
	fmt.Println("\nEstado Proceso:")
	funciones.EstadoProceso(configJson)
	fmt.Println("\nListar Procesos:")
	funciones.ListarProceso(configJson)
	fmt.Println("\nDetener Planificación:")
	//funciones.DetenerPlanificacion(configJson)
	fmt.Println("\nIniciar Planificación:")
	//funciones.IniciarPlanificacion(configJson)
}

func testPlanificacion(configJson config.Kernel) {

	printList := func() {
		fmt.Println("readyQueue:")
		var ready []uint32
		for _, pcb := range funciones.ReadyQueue {
			ready = append(ready, pcb.PID)
		}
		fmt.Println(ready)
	}

	//
	fmt.Printf("\nSe crean 2 procesos-------------\n\n")
	for i := 0; i < 2; i++ {
		path := "procesos" + strconv.Itoa(funciones.Counter) + ".txt"
		funciones.IniciarProceso(configJson, path)
	}

	fmt.Printf("\nSe testea el planificador-------------\n\n")
	funciones.Planificador(configJson)
	printList()

	fmt.Printf("\nSe crean 2 procesos-------------\n\n")
	for i := 0; i < 2; i++ {
		path := "proceso" + strconv.Itoa(funciones.Counter) + ".txt"
		funciones.IniciarProceso(configJson, path)
	}
}

func testCicloDeInstruccion(configJson config.Kernel) {

	fmt.Printf("\nSe crean 1 proceso-------------\n\n")
	funciones.IniciarProceso(configJson, "proceso_test")

	fmt.Printf("\nSe testea el planificador-------------\n\n")
	funciones.Planificador(configJson)
}
*/
