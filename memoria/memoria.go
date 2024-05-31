package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/sisoputnfrba/tp-golang/memoria/funciones"
	"github.com/sisoputnfrba/tp-golang/utils/config"
	"github.com/sisoputnfrba/tp-golang/utils/structs"
)

//================================| MAIN |================================\\

func main() {

	// Extrae info de config.json
	config.Iniciar("config.json", &funciones.ConfigJson)

	// Crea e inicializa la memoria de instrucciones
	memoriaInstrucciones := make(map[uint32][]string)

	// Configura el logger
	config.Logger("Memoria.log")

	// ======== HandleFunctions ========
	http.HandleFunc("PUT /process", handlerMemIniciarProceso(memoriaInstrucciones))
	http.HandleFunc("GET /instrucciones", handlerEnviarInstruccion(memoriaInstrucciones))

	//inicio el servidor de Memoria
	config.IniciarServidor(funciones.ConfigJson.Port)
}

//================================| HANDLERS |================================\\

// Wrapper que crea un PCB con el pid recibido.
func handlerMemIniciarProceso(memoriaInstrucciones map[uint32][]string) func(http.ResponseWriter, *http.Request) {

	// Handler para iniciar un proceso.
	return func(w http.ResponseWriter, r *http.Request) {

		//variable que recibirá la request.
		var request structs.BodyIniciarProceso

		// Decodifica en formato JSON la request.
		err := json.NewDecoder(r.Body).Decode(&request)
		if err != nil {
			fmt.Println(err) //TODO: por el momento se deja para desarrollo, eliminar al terminar el TP.
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Se guardan las instrucciones en un map de memoria.
		funciones.GuardarInstrucciones(request.PID, request.Path, memoriaInstrucciones)

		// Crea una variable tipo Response (para confeccionar una respuesta)
		var respBody structs.ResponseListarProceso = structs.ResponseListarProceso{PID: request.PID}
		respuesta, err := json.Marshal(respBody)
		if err != nil {
			fmt.Println(err) //TODO: por el momento se deja para desarrollo, eliminar al terminar el TP.
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Envía respuesta (con estatus como header) al cliente
		w.WriteHeader(http.StatusOK)
		w.Write(respuesta)
	}
}

// Envía a CPU la instrucción correspondiente al pid y el pc del map de memoria
func handlerEnviarInstruccion(memoriaInstrucciones map[uint32][]string) func(http.ResponseWriter, *http.Request) {

	// Handler para enviar una instruccion
	return func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()
		pid, errPid := strconv.ParseUint(queryParams.Get("PID"), 10, 32)
		pc, errPC := strconv.ParseUint(queryParams.Get("PC"), 10, 32)

		if errPid != nil || errPC != nil {
			return
		}

		instruccion := memoriaInstrucciones[uint32(pid)][uint32(pc)]
		fmt.Println(instruccion) //! Borrar despues

		// Esperar un tiempo determinado a tiempo de retardo
		time.Sleep(time.Duration(funciones.ConfigJson.Delay_Response) * time.Millisecond)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(instruccion))
	}
}
