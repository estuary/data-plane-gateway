package main

import (
	"context"
	"crypto/tls"
	"fmt"
	log "github.com/sirupsen/logrus"
	pb "go.gazette.dev/core/broker/protocol"
	pc "go.gazette.dev/core/consumer/protocol"
	"google.golang.org/grpc"
	"io"
	"net"
	"net/http"
)

type TlsServer struct {
	shardsDomain string
	httpsServer  *http.Server
}
