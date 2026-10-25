                    <?php } ?>
                    <?php if ($this->permission1->method('terms_add', 'create')->access()) { ?>
                        <li class="treeview <?php
                                            if ($this->uri->segment('1') == ("terms_add")) {
                                                echo "active";
                                            } else {
                                                echo " ";
                                            }
                                            ?>"><a href="<?php echo base_url('terms_add') ?>"><?php echo display('terms_add') ?></a></li>
                    <?php } ?>
                </ul>
            </li>
        <?php } ?>

        <?php if ($this->permission1->method('add_customer', 'create')->access() || $this->permission1->method('manage_customer', 'read')->access() || $this->permission1->method('credit_customer', 'read')->access() || $this->permission1->method('paid_customer', 'read')->access() || $this->permission1->method('customer_ledger', 'read')->access() || $this->permission1->method('customer_advance', 'create')->access()) { ?>
            <li
                class="treeview <?php echo (($this->uri->segment(1) == "add_customer" || $this->uri->segment(1) == "customer_list" || $this->uri->segment(1) == "credit_customer" || $this->uri->segment(1) == "paid_customer" || $this->uri->segment(1) == "edit_customer" || $this->uri->segment(1) == "customer_ledgerdata" || $this->uri->segment(1) == "customer_ledger" || $this->uri->segment(1) == "advance_receipt" || $this->uri->segment(1) == "customer_advance") ? "active" : '') ?>">

                <a href="javascript:void(0)">

                    <i class="metismenu-icon pe-7s-user"></i> <span><?php echo display('customer') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>

                <ul class="treeview-menu">

                    <?php if ($this->permission1->method('add_customer', 'create')->access()) { ?>
                        <li class="<?php echo (($this->uri->segment(1) == "add_customer") ? "active" : '') ?>">
                            <a href="<?php echo base_url('add_customer') ?>"
                                class="<?php echo (($this->uri->segment(1) == "add_customer") ? "active" : null) ?>">
                                <?php echo display('add_customer') ?>

                            </a>

                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('manage_customer', 'read')->access()) { ?>
                        <li class="<?php echo (($this->uri->segment(1) == "customer_list") ? "active" : '') ?>">
                            <a href="<?php echo base_url('customer_list') ?>">

                                <?php echo display('customer_list') ?>

                            </a>

                        </li>
                    <?php } ?>


                </ul>

            </li>
        <?php } ?>
        <!-- supplier menu part -->
        <?php if ($this->permission1->method('add_supplier', 'create')->access() || $this->permission1->method('manage_supplier', 'read')->access() || $this->permission1->method('supplier_ledger', 'read')->access() || $this->permission1->method('supplier_advance', 'create')->access()) { ?>
            <li
                class="treeview <?php echo (($this->uri->segment(1) == "add_supplier" || $this->uri->segment(1) == "supplier_list" || $this->uri->segment(1) == "edit_supplier" || $this->uri->segment(1) == "supplier_ledgerdata" || $this->uri->segment(1) == "supplier_ledger" || $this->uri->segment(1) == "supplier_advance") ? "active" : '') ?>">

                <a href="javascript:void(0)">

                    <i class="metismenu-icon fa fa-user-secret"></i> <span><?php echo display('supplier') ?></span>
                    <span class="pull-right-container">
                        <i class="fa fa-angle-left pull-right"></i>
                    </span>
                </a>

                <ul class="treeview-menu">

                    <?php if ($this->permission1->method('add_supplier', 'create')->access()) { ?>
                        <li class="<?php echo (($this->uri->segment(1) == "add_supplier") ? "active" : '') ?>">
                            <a href="<?php echo base_url('add_supplier') ?>"
                                class="<?php echo (($this->uri->segment(1) == "add_supplier") ? "active" : null) ?>">
                                <?php echo display('add_supplier') ?>

                            </a>

                        </li>
                    <?php } ?>
                    <?php if ($this->permission1->method('manage_supplier', 'read')->access()) { ?>