(function_definition
  type: (_) @ftype
  declarator: (function_declarator
    declarator: (identifier) @fname
    parameters: (parameter_list) @fparams))

(struct_specifier
  "struct" @ftype
  name: (type_identifier) @fname)
