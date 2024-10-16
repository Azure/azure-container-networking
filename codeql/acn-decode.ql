/**
 * @name Command Injection From Decode
 * @description Flow exists from decodes (untrusted) to exec command
 * @kind path-problem
 * @problem.severity error
 * @id go/cmd-inject-decode
 * @tags security
 * @security-severity 9.8
 * @precision high
 */

// Detect flow from the DECODE method (which decodes http requests) to a command execution
import go
import lib.ACN

private class Source extends DataFlow2::Node {
  Source() {
    exists(DataFlow::CallNode c |
      c.getTarget().hasQualifiedName("github.com/Azure/azure-container-networking/common", "Decode") and
      c.getArgument(2) = this
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
