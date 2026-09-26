# -*- mode: zsh; sh-indentation: 2; indent-tabs-mode: nil; sh-basic-offset: 2; -*-
# vim: ft=zsh sw=2 ts=2 et
# Manual: https://zsh.sourceforge.io/Doc/Release/Shell-Grammar.html#Alternate-Forms-For-Complex-Commands
# Issue #214: csh-style foreach ... end loops.
# Preserved as permanent regression coverage.

foreach item (one two)
  print -r -- "$item"
end

foreach a b (1 2 3 4)
  print -r -- "$a" "$b"
end

foreach v (alpha) {
  print -r -- "$v"
}

foreach v (beta) do
  print -r -- "$v"
done

foreach v in first second; print -r -- "$v"; end

foreach outer (1 2)
  foreach inner (3 4)
    print -r -- "$outer" "$inner"
  end
end

foo() {
  foreach v (fn-arg)
    print -r -- "$v"
  end
}
foo

foreach v (arg-test)
  print -r -- end
end

foreach v (redirect-test)
  print -r -- "$v"
end > /dev/null

foreach v (pipe-test)
  print -r -- "$v"
end | cat > /dev/null

foreach v (and-test)
  print -r -- "$v"
end && print -r -- ok > /dev/null
