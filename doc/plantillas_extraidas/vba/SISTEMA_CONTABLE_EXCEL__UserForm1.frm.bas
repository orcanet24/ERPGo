' Origen: SISTEMA_CONTABLE EXCEL / UserForm1.frm
Attribute VB_Name = "UserForm1"
Attribute VB_Base = "0{CDA7A102-CE66-458F-99E7-021E881105B9}{0FB870AA-2F3D-4B5D-BCE9-43E44095CED9}"
Attribute VB_GlobalNameSpace = False
Attribute VB_Creatable = False
Attribute VB_PredeclaredId = True
Attribute VB_Exposed = False
Attribute VB_TemplateDerived = False
Attribute VB_Customizable = False


Private Sub ListBox1_DblClick(ByVal Cancel As MSForms.ReturnBoolean)

fila = Cells(1, 9)

Hoja4.Cells(fila, 1).Value = ListBox1.Column(3)
Hoja4.Cells(fila, 2).Value = ListBox1.Column(4)

End Sub
