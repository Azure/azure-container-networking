/**
 * @name Call to library function
 * @description Finds calls
 * @id go/examples/calltocommand
 * @kind problem
 * @problem.severity warning
 * @tags call
 *       function
 */

import go

from Function println, DataFlow::CallNode call
where
  println.hasQualifiedName("os/exec", "Command") and
  call = println.getACall()
select call, "found cmd"
