package master

// Certificate Validity checks: what the last check saw is kept per service
// (Service.Cert, in checks.dat) for the website's "Show" view. Details from
// a remote probe are cleaned and limited like every other agent text.

import (
	"coremon/internal/probe"
	"coremon/internal/proto"
)

const (
	certInterval   = 3600 // a certificate check's default heartbeat
	maxCertChain   = 8
	maxCertNames   = 100
	maxCertText    = 300
	certTargetDNS  = "dns"    // connect to where the name points
	certTargetHost = "device" // connect to the device's own address
)

// cleanCert: a copy with every text cleaned and the lists limited.
func cleanCert(c *proto.CertInfo) *proto.CertInfo {
	if c == nil {
		return nil
	}
	out := &proto.CertInfo{Addr: cleanText(c.Addr, 80), TLS: cleanText(c.TLS, 20), Cipher: cleanText(c.Cipher, 80)}
	for i, e := range c.Chain {
		if i == maxCertChain {
			break
		}
		n := proto.CertEntry{Subject: cleanText(e.Subject, maxCertText), Issuer: cleanText(e.Issuer, maxCertText),
			NotBefore: e.NotBefore, NotAfter: e.NotAfter, Key: cleanText(e.Key, 40), SigAlg: cleanText(e.SigAlg, 40),
			Serial: cleanText(e.Serial, 80), SHA256: cleanText(e.SHA256, 100), CA: e.CA}
		for j, name := range e.Names {
			if j == maxCertNames {
				break
			}
			if v := cleanText(name, 253); v != "" {
				n.Names = append(n.Names, v)
			}
		}
		out.Chain = append(out.Chain, n)
	}
	return out
}

// certTarget: how the website names where a certificate check connects.
func (sv *Service) certTarget() string {
	if sv.Method != probe.MethodCert {
		return ""
	}
	if sv.Remote {
		return certTargetDNS
	}
	return certTargetHost
}
