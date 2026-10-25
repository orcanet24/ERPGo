' Origen: Calculos Salariales 2013 / Módulo1.bas
Attribute VB_Name = "Módulo1"
Function amensual(Tipo, monto)
    Select Case Tipo
    Case Is = "Diario"
    amensual = monto * 30
    Case Is = "Semanal"
    amensual = monto / 7 * 30
    Case Is = "Quincenal"
    amensual = monto * 2
    Case Is = "Mensual"
    amensual = monto
    End Select
End Function

'*************** Nombre de Variables **********************
'He = hora de entrada
'Hs = Hora de salida
'HeTx = Hora de entrada Turno X
'HsTx = Hora de salida Turno X'
'IHn = Inicio de hora nocturna
'Turno = Indica el turno asignado
'Hd = horas de descanso
'Thd = turno de horas de descanso
'***********************************************************



'Determina cantidad de horas diurnas trabajadas
Function horad(Turno, He, Hs, Ihn, HeT1, HsT1, HeT2, HsT2, HeT3, HsT3, Hd, Thd)

If Hs < He Then
Hs = Hs + 1
End If

Select Case Turno
    
    Case Is = "T1"
        If HsT1 <= Ihn Then
        'Para horario Diurno
            If Hs > HsT1 Then
                horad = (HsT1 - He) * 24
            Else
                horad = (Hs - He) * 24
            End If
        Else
        'Caso de horario Mixto o Nocturno
            If He < Ihn Then
            'Si El horario es mixto
                horad = (Ihn - He) * 24
            Else
            'Si El horario es Nocturno
                horad = 0
            End If
        End If
      
    Case Is = "T2"
         If HsT2 <= Ihn Then
        'Para horario Diurno
            If Hs > HsT2 Then
                horad = (HsT2 - He) * 24
            Else
                horad = (Hs - He) * 24
            End If
        Else
        'Caso de horario Mixto o Nocturno
            If He < Ihn Then
            'Si El horario es mixto
                horad = (Ihn - He) * 24
            Else
            'Si El horario es Nocturno
                horad = 0
            End If
        End If
            
    Case Is = "T3"
         If HsT3 <= Ihn Then
        'Para horario Diurno
            If Hs > HsT3 Then
                horad = (HsT3 - He) * 24
            Else
                horad = (Hs - He) * 24
            End If
        Else
        'Caso de horario Mixto o Nocturno
            If He < Ihn Then
            'Si El horario es mixto
                horad = (Ihn - He) * 24
            Else
            'Si El horario es Nocturno
                horad = 0
            End If
        End If
End Select

'Resta las horas de descanso a la jornada

If Thd = "Diurna" Then
horad = horad - Hd
End If

If horad < 0 Then
horad = 0
End If

End Function

'Calculo horas extras diurnas
Function horade(Turno, He, Hs, Ihn, HeT1, HsT1, HeT2, HsT2, HeT3, HsT3)

If Hs < He Then
Hs = Hs + 1
End If
 
Select Case Turno
    Case Is = "T1"
        If Hs > HsT1 Then
            If Hs > Ihn Then
                horade = (Ihn - HsT1) * 24
            Else
                horade = (Hs - HsT1) * 24
            End If
        End If
    Case Is = "T2"
    If Hs > HsT2 Then
            If Hs > Ihn Then
                horade = (Ihn - HsT2) * 24
            Else
                horade = (Hs - HsT2) * 24
            End If
        End If
    
    Case Is = "T3"
    If Hs > HsT3 Then
            If Hs > Ihn Then
                horade = (Ihn - HsT3) * 24
            Else
                horade = (Hs - HsT3) * 24
            End If
        End If
End Select

If He > Ihn Then
horade = 0
End If

If horade < 0 Then
horade = 0
End If

End Function

'Calculo horas nocturnas
Function horan(Turno, He, Hs, Ihn, HeT1, HsT1, HeT2, HsT2, HeT3, HsT3, Hd, Thd)

If Hs < He Then
Hs = Hs + 1
End If

If HsT1 < HeT1 Then
HsT1 = HsT1 + 1
End If

If HsT2 < HeT2 Then
HsT2 = HsT2 + 1
End If

If HsT3 < HeT3 Then
HsT3 = HsT3 + 1
End If

Select Case Turno
    Case Is = "T1"
        If HsT1 <= Ihn Then
            horan = 0
        Else
            If Hs >= HsT1 Then
                If HeT1 > Ihn Then
                    horan = (HsT1 - HeT1) * 24
                Else
                    horan = (HsT1 - Ihn) * 24
                End If
            Else
                horan = (Hs - Ihn) * 24
            End If
        End If

Case Is = "T2"
If HsT2 <= Ihn Then
            horan = 0
        Else
            If Hs >= HsT2 Then
                If HeT2 > Ihn Then
                    horan = (HsT2 - HeT2) * 24
                Else
                    horan = (HsT2 - Ihn) * 24
                End If
            Else
                horan = (Hs - Ihn) * 24
            End If
        End If

Case Is = "T3"
If HsT3 <= Ihn Then
            horan = 0
        Else
            If Hs >= HsT3 Then
                If HeT3 > Ihn Then
                    horan = (HsT3 - HeT3) * 24
                Else
                    horan = (HsT3 - Ihn) * 24
                End If
            Else
                horan = (Hs - Ihn) * 24
            End If
        End If

End Select

If Thd <> "Diurna" Then
horan = horan - Hd
End If

If horan < 0 Then
horan = 0
End If

End Function

'Calculo de horas extras nocturnas
Function horaen(Turno, He, Hs, Ihn, HeT1, HsT1, HeT2, HsT2, HeT3, HsT3)

If Hs < He Then
Hs = Hs + 1
End If

If HsT1 < HeT1 Then
HsT1 = HsT1 + 1
End If

If HsT2 < HeT2 Then
HsT2 = HsT2 + 1
End If

If HsT3 < HeT3 Then
HsT3 = HsT3 + 1
End If

Select Case Turno
    Case Is = "T1"
            If HsT1 > Ihn Then
                If Hs > HsT1 Then
                    horaen = (Hs - HsT1) * 24
                End If
            Else
                If Hs > Ihn Then
                    horaen = (Hs - Ihn) * 24
                End If
            End If
    Case Is = "T2"
            If HsT2 > Ihn Then
                If Hs > HsT2 Then
                    horaen = (Hs - HsT2) * 24
                End If
            Else
                If Hs > Ihn Then
                    horaen = (Hs - Ihn) * 24
                End If
            End If
    Case Is = "T3"
            If HsT3 > Ihn Then
                If Hs > HsT3 Then
                    horaen = (Hs - HsT3) * 24
                End If
            Else
                If Hs > Ihn Then
                    horaen = (Hs - Ihn) * 24
                End If
            End If
End Select


End Function

'Calcula los dias adicionales por cada año de servicio
'A = numero de año
'M = mes de la celda anterior, debe ser el 11

Function lottt192(a, M)
If (M = 11) * (a > 1) Then
lottt192 = (a - 1) * 2
Else
lottt192 = 0
End If
If lottt192 > 30 Then
lottt192 = 30
End If
End Function

'Determina los dias para calculo de prestaciones
'Modo 1: los trimestres se toman desde el mes de entreda del trabajador
'modo 2: los trimestres se toman segun el años fiscal (Marzo, Junio,Septiembre,Diciembre)
'Modo 3: los trimestres se toman segun el mes de entrada de la LOTTT (Julio,Octubre,Enero,Abril)
'MEn = mes de entrada del empleado
'Ma=Mes que se le debe determinar si en trimestre o no

Function diaspresta(Modo, Men, Ma)

End Function



