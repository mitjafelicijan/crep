(function_definition
  type: (_) @ftype
  declarator: (function_declarator
    declarator: [
      (identifier)
      (field_identifier)
    ] @fname
    parameters: (parameter_list) @fparams))

(class_specifier
  "class" @ftype
  name: (type_identifier) @fname)

(struct_specifier
  "struct" @ftype
  name: (type_identifier) @fname)

(namespace_definition
  "namespace" @ftype
  name: (_) @fname)
