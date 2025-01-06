/**
 * @name Command Injection From CNS Invoker
 * @description Flow exists from CNS Invoker (untrusted) to exec command
 * @kind path-problem
 * @problem.severity error
 * @id go/cmd-inject-cns-invoker
 * @tags security
 * @security-severity 9.8
 * @precision high
 */

// Detect inputs from CNS Invoker to command injection
// Does not detect flow to outside the enclosed method (which is why we analyze addIpamInvoker's results too)
import go
import lib.ACN

private class Source extends DataFlow::Node {
  Source() {
    exists(DataFlow::CallNode c, Method m |
      (
        m.hasQualifiedName("github.com/Azure/azure-container-networking/cns/client", "Client",
          "RequestIPs") or
        m.hasQualifiedName("github.com/Azure/azure-container-networking/cns/client", "Client",
          "RequestIPAddress") or
        m.hasQualifiedName("github.com/Azure/azure-container-networking/cns/client", "Client",
          "GetNetworkContainer") or
        m.hasQualifiedName("github.com/Azure/azure-container-networking/cns/client", "Client",
          "GetAllNetworkContainers")
      ) and
      c = m.getACall() and
      c.getResult(0) = this
    )
  }
}

module MyConfiguration implements DataFlow::ConfigSig {
  predicate isSink(DataFlow::Node sink) { sink instanceof ACN::CommandSink }

  predicate isSource(DataFlow::Node source) { source instanceof Source }
}

module Flow = TaintTracking::Global<MyConfiguration>;

import Flow::PathGraph

from Flow::PathNode source, Flow::PathNode sink
where Flow::flowPath(source, sink)
select sink.getNode(), source, sink, "potential command injection"
