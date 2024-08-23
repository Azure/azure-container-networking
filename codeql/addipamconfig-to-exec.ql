/**
 * @name Command Injection From CNS ipam add result / CNS multitenancy ipam add result
 * @description Flow exists from CNS ipam add result / CNS multitenancy ipam add result (untrusted) to exec command
 * @kind path-problem
 * @problem.severity error
 * @id go/cmd-inject-ipam-add-result
 * @tags security
 * @security-severity 9.8
 * @precision high
 */

// Detect inputs from CNS add ipam result / CNS multitenancy ipam add result to command injection
import go

private class Sink extends DataFlow2::Node {
  Sink() {
    exists(DataFlow::CallNode c |
      c.getTarget().hasQualifiedName("os/exec", "CommandContext") and
      (c.getArgument(2) = this or c.getArgument(1) = this)
      or
      c.getTarget().hasQualifiedName("os/exec", "Command") and
      (c.getArgument(0) = this or c.getArgument(1) = this)
    )
  }
}

private class Source extends DataFlow2::Node {
  Source() {
    exists(DataFlow::CallNode c, Method m |
      //m.hasQualifiedName("github.com/Azure/azure-container-networking/cni/network", "NetPlugin",
      //  "addIpamInvoker") or // this is not necessary since we call GetAllNetworkContainers right next to this = duplicated results, but if this call moves, uncomment this
      m.hasQualifiedName("github.com/Azure/azure-container-networking/cni/network", "Multitenancy",
        "GetAllNetworkContainers") and
      c = m.getACall() and
      c.getResult(0) = this
    )
  }
}

module MyConfiguration implements DataFlow::ConfigSig {
  predicate isSink(DataFlow::Node sink) { sink instanceof Sink }

  predicate isSource(DataFlow::Node source) { source instanceof Source }
}

module Flow = TaintTracking::Global<MyConfiguration>;

import Flow::PathGraph

from Flow::PathNode source, Flow::PathNode sink
where Flow::flowPath(source, sink)
select sink.getNode(), source, sink, "potential command injection"
