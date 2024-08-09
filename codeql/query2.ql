/**
 * @name Call to library function
 * @description Finds calls
 * @id go/examples/calltocommandctx
 * @kind problem
 * @problem.severity error
 * @tags call
 *       function
 */

import go

from Function println, DataFlow::CallNode call
where
  println.hasQualifiedName("os/exec", "CommandContext") and
  call = println.getACall()
select call, "found cmd context"
