(procedure_declaration
  (identifier) @fname
  (procedure
    "proc" @ftype
    (parameters) @fparams))

(const_declaration
  (identifier) @fname
  (procedure
    "proc" @ftype
    (parameters) @fparams))

(var_declaration
  (identifier) @fname
  (procedure
    "proc" @ftype
    (parameters) @fparams))

(struct_declaration
  (identifier) @fname
  "struct" @ftype)

(enum_declaration
  (identifier) @fname
  "enum" @ftype)
