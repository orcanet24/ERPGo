' Origen: Calculos Salariales 2013 / Hoja1.cls
Attribute VB_Name = "Hoja1"
Attribute VB_Base = "0{00020820-0000-0000-C000-000000000046}"
Attribute VB_GlobalNameSpace = False
Attribute VB_Creatable = False
Attribute VB_PredeclaredId = True
Attribute VB_Exposed = True
Attribute VB_TemplateDerived = False
Attribute VB_Customizable = True
Private Sub Worksheet_Activate()
'abre la pagina de actualizacion si es 15 del mes
'muestra un mensaje si la hoja main se ha alterado
If Range("B24").Value <> "Por: DORELIRA" Then
MsgBox "Se ha detectado que la hoja fue alterada, eso puede ocurrir debido a que no ha comprado el programa por medio del vendedor autorizado en mercadolibre DORELIRA, NO se garantiza el buen funcionamiento ni soporte de esta hoja.", vbExclamation
End If
End Sub



