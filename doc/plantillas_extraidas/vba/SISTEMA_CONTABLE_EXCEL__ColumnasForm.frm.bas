' Origen: SISTEMA_CONTABLE EXCEL / ColumnasForm.frm
Attribute VB_Name = "ColumnasForm"
Attribute VB_Base = "0{47E1859D-C2CC-461D-A590-3895432557FB}{3782CC9B-26B7-4D04-AF6A-9E24E28F7AFF}"
Attribute VB_GlobalNameSpace = False
Attribute VB_Creatable = False
Attribute VB_PredeclaredId = True
Attribute VB_Exposed = False
Attribute VB_TemplateDerived = False
Attribute VB_Customizable = False

Private Sub ColumnasLista_DblClick(ByVal Cancel As MSForms.ReturnBoolean)

fila = Cells(1, 10)

Hoja4.Cells(fila, 6).Value = ColumnasLista.Column(0)

End Sub
