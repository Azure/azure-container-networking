/**
 * @name Command Injection From CNI Args
 * @description Flow exists from CNI Args (untrusted) to exec command
 * @kind path-problem
 * @problem.severity error
 * @id go/cmd-inject-cni
 * @tags security
 * @security-severity 9.8
 * @precision high
 */

// Detect inputs from CNI ARGS to command injection
import go
import lib.ACN

private class Source extends DataFlow::Node {
  Source() {
    exists(DataFlow::CallNode c, Method m |
      (
        m.hasQualifiedName("github.com/Azure/azure-container-networking/cni/network", "NetPlugin",
          "Add") or
        m.hasQualifiedName("github.com/Azure/azure-container-networking/cni/network", "NetPlugin",
          "Delete") or
        m.hasQualifiedName("github.com/Azure/azure-container-networking/cni/network", "NetPlugin",
          "Update") or
        m.hasQualifiedName("github.com/Azure/azure-container-networking/cni/network", "NetPlugin",
          "Get")
      ) and
      c = m.getACall() and
      c.getArgument(0) = this
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
