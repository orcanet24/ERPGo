' Origen: SISTEMA_CONTABLE EXCEL / Proveedores.frm
Attribute VB_Name = "Proveedores"
Attribute VB_Base = "0{EFE53DDD-F687-462D-B9A4-D27155FF261C}{394DB737-B9FC-4ED6-92C0-8924442061F3}"
Attribute VB_GlobalNameSpace = False
Attribute VB_Creatable = False
Attribute VB_PredeclaredId = True
Attribute VB_Exposed = False
Attribute VB_TemplateDerived = False
Attribute VB_Customizable = False

Private Sub ProveedoresLis_DblClick(ByVal Cancel As MSForms.ReturnBoolean)

Hoja4.Range("h31").Value = ProveedoresLis.Column(0)
Hoja4.Range("b31").Value = ProveedoresLis.Column(1)
Hoja4.Range("c31").Value = ProveedoresLis.Column(2)

End Sub
