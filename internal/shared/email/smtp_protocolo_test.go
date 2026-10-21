package email

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Cada cliente corre en otro proceso para probar el trust store normal del
// sistema con una CA temporal, sin desactivar la verificación de certificados.
func TestSMTPClienteAuxiliar(t *testing.T) {
	if os.Getenv("SGRC_SMTP_AUXILIAR") != "1" {
		t.Skip("auxiliar del ensayo SMTP")
	}
	enviador, err := DesdeEntorno(os.Getenv, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = enviador.Enviar(ctx, "destino@test.local", "Prueba con acentos: reparación", "Línea uno\n.punto inicial\nÚltima línea")
	esperado := os.Getenv("SGRC_SMTP_ERROR")
	if esperado == "" {
		if err != nil {
			t.Fatal(err)
		}
	} else if err == nil || !strings.Contains(err.Error(), esperado) {
		t.Fatalf("error=%v, esperado=%s", err, esperado)
	}
}

type dialogoSMTP struct {
	comandos []string
	mensaje  string
	err      error
}

func certificadoSMTP(t *testing.T) (tls.Certificate, string) {
	t.Helper()
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "SGRC CA de prueba"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &clave.PublicKey, clave)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "localhost"}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &clave.PublicKey, clave)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(clave)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		t.Fatal(err)
	}
	archivo := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(archivo, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0600); err != nil {
		t.Fatal(err)
	}
	return cert, archivo
}

func servirSMTP(conn net.Conn, cert tls.Certificate, modo string) (d dialogoSMTP) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	if modo == "sin-saludo" {
		_, d.err = io.Copy(io.Discard, conn)
		return d
	}
	r := textproto.NewReader(bufio.NewReader(conn))
	w := textproto.NewWriter(bufio.NewWriter(conn))
	if err := w.PrintfLine("220 localhost SMTP"); err != nil {
		d.err = err
		return d
	}
	seguro := false
	for {
		linea, err := r.ReadLine()
		if err != nil {
			if err != io.EOF {
				d.err = err
			}
			return d
		}
		d.comandos = append(d.comandos, linea)
		switch {
		case strings.HasPrefix(linea, "EHLO "):
			if modo == "sin-tls" {
				err = w.PrintfLine("250 localhost")
			} else if seguro {
				err = w.PrintfLine("250-localhost\r\n250 AUTH PLAIN")
			} else {
				err = w.PrintfLine("250-localhost\r\n250 STARTTLS")
			}
		case linea == "STARTTLS":
			err = w.PrintfLine("220 Listo para TLS")
			if err != nil {
				d.err = err
				return d
			}
			tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
			if err := tlsConn.Handshake(); err != nil {
				d.err = err
				return d
			}
			conn = tlsConn
			r = textproto.NewReader(bufio.NewReader(conn))
			w = textproto.NewWriter(bufio.NewWriter(conn))
			seguro = true
		case strings.HasPrefix(linea, "AUTH PLAIN "):
			credenciales, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(linea, "AUTH PLAIN "))
			if !seguro || string(credenciales) != "\x00origen@test.local\x00clave-prueba" {
				d.err = fmt.Errorf("AUTH inesperado o sin TLS")
				return d
			}
			if modo == "auth-rechazada" {
				err = w.PrintfLine("535 Credenciales rechazadas")
			} else {
				err = w.PrintfLine("235 Autenticado")
			}
		case strings.HasPrefix(linea, "MAIL FROM:"):
			err = w.PrintfLine("250 Remitente aceptado")
		case strings.HasPrefix(linea, "RCPT TO:"):
			if modo == "destino-rechazado" {
				err = w.PrintfLine("550 Destino rechazado")
			} else {
				err = w.PrintfLine("250 Destinatario aceptado")
			}
		case linea == "DATA":
			if !seguro {
				d.err = fmt.Errorf("DATA sin TLS")
				return d
			}
			if err = w.PrintfLine("354 Enviar mensaje"); err != nil {
				d.err = err
				return d
			}
			var datos []byte
			datos, err = r.ReadDotBytes()
			d.mensaje = string(datos)
			if err == nil {
				err = w.PrintfLine("250 Mensaje recibido")
			}
		case linea == "QUIT":
			d.err = w.PrintfLine("221 Adiós")
			return d
		default:
			err = w.PrintfLine("500 Comando desconocido")
		}
		if err != nil {
			d.err = err
			return d
		}
	}
}

func TestSMTP_ProtocoloTLSYErrores(t *testing.T) {
	cert, ca := certificadoSMTP(t)
	ejecutable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, caso := range []struct{ modo, esperado string }{
		{"correcto", ""}, {"sin-tls", "no ofrece STARTTLS"}, {"auth-rechazada", "autenticando"}, {"destino-rechazado", "destinatario rechazado"}, {"ca-no-confiable", "negociando TLS"}, {"sin-saludo", "saludo SMTP"},
	} {
		t.Run(caso.modo, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			resultado := make(chan dialogoSMTP, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					resultado <- dialogoSMTP{err: err}
					return
				}
				resultado <- servirSMTP(conn, cert, caso.modo)
			}()
			_, puerto, err := net.SplitHostPort(listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, ejecutable, "-test.run=^TestSMTPClienteAuxiliar$")
			cmd.Env = append(os.Environ(), "SGRC_SMTP_AUXILIAR=1", "SGRC_SMTP_ERROR="+caso.esperado, "SMTP_HOST=127.0.0.1", "SMTP_PORT="+puerto, "SMTP_USER=origen@test.local", "SMTP_PASSWORD=clave-prueba", "SMTP_FROM=origen@test.local", "SSL_CERT_DIR="+t.TempDir())
			if caso.modo == "ca-no-confiable" {
				cmd.Env = append(cmd.Env, "SSL_CERT_FILE=/no-existe.pem")
			} else {
				cmd.Env = append(cmd.Env, "SSL_CERT_FILE="+ca)
			}
			if salida, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("cliente: %v\n%s", err, salida)
			}
			var d dialogoSMTP
			select {
			case d = <-resultado:
			case <-ctx.Done():
				t.Fatal("el servidor de prueba no terminó")
			}
			if caso.modo != "correcto" {
				if d.mensaje != "" {
					t.Fatal("un envío rechazado transmitió DATA")
				}
				if caso.modo == "sin-tls" || caso.modo == "ca-no-confiable" {
					for _, c := range d.comandos {
						if strings.HasPrefix(c, "AUTH ") {
							t.Fatal("se envió AUTH antes de validar TLS")
						}
					}
				}
				return
			}
			if d.err != nil {
				t.Fatal(d.err)
			}
			comandos := strings.Join(d.comandos, "\n")
			if !strings.Contains(comandos, "MAIL FROM:<origen@test.local>") || !strings.Contains(comandos, "RCPT TO:<destino@test.local>") {
				t.Fatal("sobre SMTP incorrecto")
			}
			mensaje, err := mail.ReadMessage(strings.NewReader(d.mensaje))
			if err != nil {
				t.Fatal(err)
			}
			asunto, err := new(mime.WordDecoder).DecodeHeader(mensaje.Header.Get("Subject"))
			if err != nil || asunto != "Prueba con acentos: reparación" {
				t.Fatalf("asunto=%q, error=%v", asunto, err)
			}
			cuerpo, err := io.ReadAll(quotedprintable.NewReader(mensaje.Body))
			if err != nil || strings.ReplaceAll(string(cuerpo), "\r\n", "\n") != "Línea uno\n.punto inicial\nÚltima línea\n" {
				t.Fatalf("cuerpo=%q, error=%v", cuerpo, err)
			}
		})
	}
}
