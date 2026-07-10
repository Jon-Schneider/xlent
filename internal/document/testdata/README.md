# VBA round-trip fixture

`real-vba-project.xlsm` contains Excelize's `test/vbaProject.bin` fixture,
added with Excelize v2.10.1. It is used only to verify that xlent preserves a
real VBA project when a macro-enabled workbook is opened and saved; xlent
never executes macros.

The fixture is distributed under the BSD 3-Clause License:

Copyright (c) 2016-2026 The excelize Authors.
Copyright (c) 2011-2017 Geoffrey J. Teale.

See the full license in the Excelize dependency's `LICENSE` file.
