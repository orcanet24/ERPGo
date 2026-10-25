' Origen: SISTEMA_CONTABLE EXCEL / ClientesForm.frm
Attribute VB_Name = "ClientesForm"
Attribute VB_Base = "0{DF341C51-DBF7-43A3-A7A0-333130722EB4}{9F74FF25-0764-406D-BEC5-9F9D0F51BA0C}"
Attribute VB_GlobalNameSpace = False
Attribute VB_Creatable = False
Attribute VB_PredeclaredId = True
Attribute VB_Exposed = False
Attribute VB_TemplateDerived = False
Attribute VB_Customizable = False

Private Sub ClientesList_DblClick(ByVal Cancel As MSForms.ReturnBoolean)

Hoja4.Range("h31").Value = ClientesList.Column(0)
Hoja4.Range("b31").Value = ClientesList.Column(1)
Hoja4.Range("c31").Value = ClientesList.Column(2)

End Sub
