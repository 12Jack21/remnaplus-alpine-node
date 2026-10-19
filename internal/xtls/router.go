package xtls

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	router "github.com/xtls/xray-core/app/router"
	routercommand "github.com/xtls/xray-core/app/router/command"
	"github.com/xtls/xray-core/common/serial"
	"google.golang.org/grpc"
)

type RouterResult struct {
	OK      bool
	Message string
}

type RouterAPI struct {
	client routercommand.RoutingServiceClient
}

func NewRouterAPI(conn *grpc.ClientConn) *RouterAPI {
	return &RouterAPI{client: routercommand.NewRoutingServiceClient(conn)}
}

func (r *RouterAPI) AddSourceIPRule(ctx context.Context, ruleTag, outbound, ip string, appendRule bool) RouterResult {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return RouterResult{Message: "invalid IP address"}
	}
	var prefix uint32
	if parsed.To4() != nil {
		parsed = parsed.To4()
		prefix = 32
	} else {
		prefix = 128
	}
	_, err := r.client.AddRule(ctx, &routercommand.AddRuleRequest{
		Config: serial.ToTypedMessage(&router.Config{
			Rule: []*router.RoutingRule{{
				RuleTag:     ruleTag,
				TargetTag:   &router.RoutingRule_Tag{Tag: outbound},
				SourceGeoip: []*router.GeoIP{{Cidr: []*router.CIDR{{Ip: parsed, Prefix: prefix}}}},
			}},
		}),
		ShouldAppend: appendRule,
	})
	if err != nil {
		return RouterResult{Message: grpcErrorMessage(err)}
	}
	return RouterResult{OK: true}
}

func (r *RouterAPI) RemoveRuleByTag(ctx context.Context, ruleTag string) RouterResult {
	_, err := r.client.RemoveRule(ctx, &routercommand.RemoveRuleRequest{RuleTag: ruleTag})
	if err != nil {
		return RouterResult{Message: grpcErrorMessage(err)}
	}
	return RouterResult{OK: true}
}

func SourceIPRuleTag(ip string) string {
	sum := md5.Sum([]byte("string:" + fmt.Sprint(len(ip)) + ":" + ip))
	return hex.EncodeToString(sum[:])
}
