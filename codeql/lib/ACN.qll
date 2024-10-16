import go

module ACN {
  class CommandSink extends DataFlow2::Node {
    CommandSink() {
      exists(DataFlow::CallNode c, Method m |
        (
          // Detect dangerous usage of command wrappers with the command in the 0th arg position
          (
            m.hasQualifiedName("github.com/Azure/azure-container-networking/platform", "execClient",
              "ExecuteRawCommand") or
            m.hasQualifiedName("github.com/Azure/azure-container-networking/platform", "execClient",
              "ExecutePowershellCommand")
          ) and
          c.getArgument(0) = this
          or
          // Detect dangerous usage of command wrappers with the command in the 1st arg position
          m.hasQualifiedName("github.com/Azure/azure-container-networking/platform", "execClient",
            "ExecutePowershellCommandWithContext") and
          c.getArgument(1) = this
        ) and
        c = m.getACall()
        or
        // Detect dangerous calls directly to os exec
        (
          c.getTarget().hasQualifiedName("os/exec", "CommandContext") and
          (c.getArgument(2) = this or c.getArgument(1) = this)
          or
          c.getTarget().hasQualifiedName("os/exec", "Command") and
          (c.getArgument(0) = this or c.getArgument(1) = this)
        )
      )
    }
  }
}
