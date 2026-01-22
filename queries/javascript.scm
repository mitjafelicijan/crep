(function_declaration
  "function" @ftype
  name: (identifier) @fname
  parameters: (formal_parameters) @fparams)

(method_definition
  name: [(property_identifier) (identifier)] @fname
  parameters: (formal_parameters) @fparams)

(class_declaration
  "class" @ftype
  name: (identifier) @fname)
